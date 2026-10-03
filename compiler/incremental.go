package compiler

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/shikejing/material-graph-compiler-go/graph"
	"github.com/shikejing/material-graph-compiler-go/nodelib"
)

// LineCache 缓存“节点依赖哈希 -> 该节点生成的代码行”，供增量编译跨请求复用。
// 实现必须并发安全（cache.Cache[string] 即满足）。
type LineCache interface {
	// Peek 只读查询，不触发计算。
	Peek(key string) (string, bool)
	// GetOrCompute 未命中时计算并写入。
	GetOrCompute(key string, compute func() (string, error)) (string, error)
}

// IncrementalResult 是增量编译的结果。
type IncrementalResult struct {
	Program   *Program
	Reused    []graph.NodeID          // 代码行被复用的节点
	Rebuilt   []graph.NodeID          // 重新生成代码行的节点
	DepHashes map[graph.NodeID]string // 每个节点的依赖哈希，供诊断与测试
}

// CompileIncremental 增量编译：只重新生成受影响节点的代码行，
// 未变化节点的代码行从 lineCache 复用。最终 Source 与 Compile 全量编译
// 逐字节一致：每行内容只由节点自身与其依赖的内容决定，装配顺序同为确定拓扑序。
func CompileIncremental(g *graph.Graph, lib *nodelib.Library, version int, opts Options, lineCache LineCache) (*IncrementalResult, error) {
	if verr := Validate(g, lib, version); verr != nil {
		return nil, verr
	}
	work := g
	var elims []Elimination
	if opts.Optimize {
		work, elims = optimize(g, lib, version)
	}
	order, err := topoSort(work)
	if err != nil {
		return nil, err
	}

	libKey := lib.VersionKey(version)
	depHashes := make(map[graph.NodeID]string, len(order))
	lines := make(map[graph.NodeID]string, len(order))
	var reused, rebuilt []graph.NodeID

	// 每个输入端口的连线来源（按端口排序，确定）。
	inEdges := make(map[graph.NodeID][]graph.Edge)
	for _, e := range work.Edges {
		inEdges[e.To.Node] = append(inEdges[e.To.Node], e)
	}
	for id := range inEdges {
		sort.Slice(inEdges[id], func(i, j int) bool { return inEdges[id][i].To.Port < inEdges[id][j].To.Port })
	}

	for _, id := range order {
		n := findNode(work, id)
		def, _ := lib.Def(version, n.Type)
		// 依赖哈希 = 节点内容 + 各输入来源的依赖哈希 + 库版本。
		h := fnv.New64a()
		fmt.Fprintf(h, "lib=%s;node=%s", libKey, graph.NodeKey(*n))
		for _, e := range inEdges[id] {
			fmt.Fprintf(h, ";in %s<-%s.%s:%s", e.To.Port, e.From.Node, e.From.Port, depHashes[e.From.Node])
		}
		dep := fmt.Sprintf("%x", h.Sum64())
		depHashes[id] = dep

		if line, ok := lineCache.Peek(dep); ok {
			lines[id] = line
			reused = append(reused, id)
			continue
		}
		node := *n
		nodeDef := def
		line, err := lineCache.GetOrCompute(dep, func() (string, error) {
			return emitNode(work, node, nodeDef, lib, version)
		})
		if err != nil {
			return nil, err
		}
		lines[id] = line
		rebuilt = append(rebuilt, id)
	}

	var sb strings.Builder
	sb.WriteString("// material program\n")
	fmt.Fprintf(&sb, "// library %s\n", libKey)
	for _, id := range order {
		sb.WriteString(lines[id])
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "output = %s;\n", varName(g.OutputNode, g.OutputPort))

	return &IncrementalResult{
		Program:   &Program{Source: sb.String(), Eliminated: elims, Lines: lines, Order: order},
		Reused:    reused,
		Rebuilt:   rebuilt,
		DepHashes: depHashes,
	}, nil
}

// emitNode 生成单个节点的全部代码行（每个输出端口一行）。
func emitNode(g *graph.Graph, n graph.Node, def *nodelib.NodeDef, lib *nodelib.Library, version int) (string, error) {
	inputs := make(map[graph.PortID]string, len(def.Inputs))
	for _, pd := range def.Inputs {
		expr, err := inputExpr(g, n, pd, lib, version)
		if err != nil {
			return "", err
		}
		inputs[pd.Name] = expr
	}
	var nodeLines []string
	for _, out := range def.Outputs {
		expr, err := def.Emit(out.Name, inputs, n.Overrides)
		if err != nil {
			return "", fmt.Errorf("node %q: emit failed: %w", n.ID, err)
		}
		nodeLines = append(nodeLines, fmt.Sprintf("let %s = %s;", varName(n.ID, out.Name), expr))
	}
	return strings.Join(nodeLines, "\n"), nil
}

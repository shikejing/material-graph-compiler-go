# Material Graph Compiler (Go)

A deterministic, local material-graph to shader compiler. It compiles a graph
of typed nodes and edges into a GLSL shader program, managing compilation
results, dependencies, and caching entirely in-process. No external services are
required; conclusions and errors are reproducible locally.

## Features

- **Graph modeling & type checking**: typed input/output ports with defaults;
  edge endpoints must be type-compatible. Cycles, edges to non-existent ports,
  and type mismatches reject the entire graph atomically, locating the node and
  port.
- **Deterministic output**: the same graph produces byte-identical shader code
  regardless of node traversal order or concurrency. No random identifiers,
  timestamps, or order-dependent content.
- **Compile-time optimization**: constant folding and dead-code elimination.
  Semantics are preserved (folded nodes become literals; eliminated nodes are
  unreachable from the output). Diagnostics explain why nodes were folded or
  eliminated.
- **Incremental recompilation**: local edits recompile only affected nodes;
  unchanged nodes are reused from a content-addressed fragment cache. The
  incremental output is byte-identical to a full recompile, and the report
  lists which nodes were reused vs recompiled.
- **Node-library versioning & compatibility**: node definitions can be upgraded;
  old graphs migrate via declared rules (port rename, add, remove, default
  change). Non-migratable upgrades are rejected. Already-produced results are
  preserved (cache keys include the library version).
- **Concurrency & result caching**: concurrent requests for the same graph
  share a single compilation (singleflight). Cache keys depend only on graph
  content and library version. Concurrent reads/writes never return half-baked
  results. Caches are bounded (LRU) so memory does not grow unboundedly.
- **Partial failure & diagnostics**: batch compilation isolates failures per
  graph. Diagnostics are structured, locatable to node/port, and
  distinguishable. Success and failure results are both queryable.

## Installation

```bash
go get github.com/shikejing/material-graph-compiler-go
```

## Quick Start

```go
import "github.com/shikejing/material-graph-compiler-go/compiler"

lib := compiler.DefaultLibrary()
s := compiler.NewSession(lib, 64, 64)

g := compiler.NewGraph()
g.AddNode(&compiler.Node{ID: "c1", Type: "constant", Inputs: map[string]compiler.Value{"value": compiler.FloatValue(0.5)}})
g.AddNode(&compiler.Node{ID: "out", Type: compiler.OutputNodeType})
g.AddEdge(&compiler.Edge{FromNode: "c1", FromPort: "out", ToNode: "out", ToPort: "baseColor"})
g.OutputNode = "out"

res := s.Compile(g)
if !res.Success {
    // inspect res.Diagnostics
}
fmt.Println(res.Program.Code)
```

## Compilation Semantics

### Graph Model

A material graph consists of:

- **Nodes**: each has a unique ID, a node type (from the library), an optional
  `TypeVersion` (the version it was authored against), and a map of explicit
  input overrides (`Inputs`).
- **Edges**: directed connections from an output port of one node to an input
  port of another. Each input port has at most one incoming edge.
- **Output node**: the designated root node (type `output`) whose result is the
  shader's return value.

### Type Compatibility

An output port of type `from` may connect to an input port of type `to` when:

- `from == to`, or
- `color` and `vec3` (interchangeable GLSL representations), or
- `int` coerces to `float`, `vec2`, `vec3`, `vec4`, or
- `float` coerces to `vec2`, `vec3`, `vec4` (splat).

Constant values are coerced to the port type during folding and emission.
Connected inputs are wrapped in GLSL coercion casts when the source type differs
from the port type.

### Validation

`ValidateGraph` checks the graph against the library and returns all
diagnostics. The graph is accepted only if no error diagnostic is present.
Checks include:

- node types exist in the library;
- input ports exist and value types are compatible;
- edges connect to existing, type-compatible ports;
- each input port is connected at most once;
- the output node is valid and unique;
- no cycles (DFS with coloring).

Cycles are reported with the involved nodes and ports. The entire graph is
rejected atomically — no partial output is produced.

### Migration

`MigrateGraph` upgrades every node to the library's current node versions using
declared `Migration` rules:

- **Port rename**: input overrides and edges are rewritten to the new port name.
- **Output rename**: edges from the node's output ports are rewritten.
- **Port removal**: if the graph overrides the removed port (explicit value or
  incoming edge), migration fails and the graph is rejected. Otherwise the port
  is dropped.
- **Port addition**: new ports use the current definition's default.
- **Default change**: non-overridden ports use the new default; overrides are
  preserved.

Migrations must form a complete path from the node's authored version to the
current version. Gaps or newer-than-library versions are rejected.

### Code Generation

The compiler emits a GLSL `material()` function. Nodes are processed in a
deterministic topological order (Kahn's algorithm with a sorted ready set,
breaking ties by node ID). Variable names are derived from node ID and port
name, so they are stable across recompiles and independent of traversal order.

Folded nodes emit no statements (their outputs are literals). The output node's
`result` variable is returned.

## Optimization Strategy

### Constant Folding

A node is constant when all its input ports are constants (literal overrides,
defaults, or connected to folded constant nodes). Constant nodes with an `Eval`
function are evaluated and replaced by their literal values. Folding preserves
semantics: `Eval` computes the same value the emitted GLSL would produce.

Folding is applied in topological order so upstream folded values are available
to downstream nodes.

### Dead-Code Elimination

A node is live if it is reachable backward from the output node (following
incoming edges). Nodes not reachable from the output are eliminated. Eliminated
nodes have no effect on the output, so removing them preserves semantics. Each
eliminated node is reported with an `eliminated` diagnostic explaining the
reason (unreachable).

### Semantic Preservation

Both optimizations are semantics-preserving:

- Folding replaces a computation with its literal result (identical value).
- DCE removes only nodes that cannot affect the output.

The observable input/output contract is unchanged.

## Cache Invalidation Rules

### Result Cache

- **Key**: `SHA-256(graph.Canonical() + "|lib" + libVersion)`.
- The key depends only on graph content (nodes, edges, values, output node) and
  the node library version.
- Any change to the graph content or library version changes the key → cache
  miss → recompile.
- Library version bumps create new keys; old results remain cached and
  queryable (they are not invalidated).
- Bounded (LRU): when capacity is exceeded, the least recently used entries are
  evicted.

### Fragment Cache

- **Key**: `SHA-256(nodeID + ":" + nodeType + ":" + version + ":" + inputBindings)`.
- Input bindings are constant values (`const:<canonical>`) or upstream fragment
  keys (`frag:<upstreamKey>`).
- The key is content-addressed: unchanged nodes (and their unchanged inputs)
  produce the same key → cache hit → reuse.
- A change to any upstream node changes its fragment key, which changes
  downstream fragment keys → downstream nodes are recompiled.
- The node ID is included in the key so fragments are only reused within the
  same graph (where IDs are stable), preventing incorrect cross-graph reuse.
- Bounded (LRU).

### Singleflight

Concurrent requests for the same graph key share a single compilation. The first
request compiles; concurrent requests wait for the result. After completion,
the result is cached and subsequent requests hit the cache. This prevents
duplicate computation under concurrency.

### Determinism Guarantees

- No random identifiers, timestamps, or UUIDs in keys or output.
- Graph canonicalization sorts nodes and edges.
- Topological order is deterministic (sorted tie-breaking).
- Variable names are ID-based.
- Cache keys are content-addressed.

## Diagnostics

Diagnostics are structured with:

- **Severity**: error, warning, info.
- **Kind**: cycle, port_not_found, type_mismatch, unknown_node_type,
  migration_failed, invalid_parameter, eliminated, folded, reused, recompiled.
- **NodeID** and **Port**: locatable to the specific node and port.
- **Message**: human-readable explanation.

`CompileResult` includes the program (on success), diagnostics, and a report
listing reused, recompiled, eliminated, and folded nodes.

## Batch Compilation

`BatchCompile` compiles each graph independently. A failure in one graph does
not affect the others. Results are returned in the same order as the input, each
with its own success/failure status and diagnostics.

## Testing

```bash
go test ./...
```

Tests cover: cycle detection, self-loops, type mismatch, port not found,
unknown node type, no/multiple output, double connection, constant folding,
dead-code elimination, determinism (insertion order and concurrency),
incremental consistency (node added/removed/changed), migration (rename, add,
remove, default change, rejection), singleflight, LRU eviction, bounded fragment
cache, no half-baked results, and partial batch failure.

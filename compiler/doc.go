// Package compiler implements a deterministic, local material-graph to shader
// compiler. It supports graph validation, constant folding, dead-code
// elimination, incremental recompilation via content-addressed fragment
// caching, node-library versioning with deterministic migration, and
// concurrent compilation with bounded caches.
package compiler

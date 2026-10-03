package nodelib

import "hash/fnv"

// fnvNew 返回一个 64 位 FNV-1a 哈希器（包内统一入口，便于替换）。
func fnvNew() interface {
	Write(p []byte) (n int, err error)
	Sum64() uint64
} {
	return fnv.New64a()
}

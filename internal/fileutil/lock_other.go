//go:build !unix

package fileutil

import "sync"

// locks は flock が無い環境でのプロセス内排他。
var locks sync.Map

// Lock は path をプロセス内で排他する。呼び出し側は unlock を必ず呼ぶ。
func Lock(path string) (unlock func(), err error) {
	v, _ := locks.LoadOrStore(path, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock, nil
}

package fileutil

import (
	"os"
	"path/filepath"
)

// AtomicWrite はアトミックにファイルを書き込む。
// 一時ファイルに書き出してから rename することでデータの整合性を保証する。
func AtomicWrite(dest string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(dest)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpName)
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		os.Remove(tmpName)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dest)
}

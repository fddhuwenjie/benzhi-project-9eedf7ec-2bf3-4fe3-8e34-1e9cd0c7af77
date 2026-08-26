//go:build !cgo

package store

// This file provides the persistence hooks used by persistence.go when the
// application is built with CGO disabled (for example, the portable
// multi-architecture container build).  The regular build uses the SQLite C
// API in sqlite_cgo.go.  In a CGO-free binary we retain the same durable
// semantics by storing the serialized aggregate in a sidecar file next to the
// requested database path.  Keeping the sidecar separate means a database
// created by a CGO-enabled process is never accidentally overwritten by the
// fallback implementation.

import (
	"fmt"
	"os"
	"path/filepath"
)

func nocgoStatePath(path string) string { return path + ".state" }

func sqliteInit(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("初始化存储目录失败: %w", err)
		}
	}
	// Touch the sidecar so Open has the same create-on-first-use behaviour as
	// SQLite.  Ignore an existing file.
	f, err := os.OpenFile(nocgoStatePath(path), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("初始化存储失败: %w", err)
	}
	return f.Close()
}

func sqliteLoad(path string) ([]byte, error) {
	if path == "" || path == ":memory:" {
		return nil, nil
	}
	b, err := os.ReadFile(nocgoStatePath(path))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取存储失败: %w", err)
	}
	return b, nil
}

func sqliteSave(path string, data []byte) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	target := nocgoStatePath(path)
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".engine-state-*")
	if err != nil {
		return fmt.Errorf("写入存储失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if chmodErr := tmp.Chmod(0o600); chmodErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入存储失败: %w", chmodErr)
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写入存储失败: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("提交存储失败: %w", err)
	}
	return nil
}

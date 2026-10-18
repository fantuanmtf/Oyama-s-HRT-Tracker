package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// atomicWrite publishes data at path without ever replacing an existing file.
//
// Steps:
//  1. create a mode-0600 temp file in the destination directory;
//  2. chmod to the requested mode, write, fsync, close;
//  3. publish with os.Link, which fails with an exists error if path exists;
//  4. remove the temp name;
//  5. best-effort fsync the destination directory.
//
// It deliberately does not fall back to rename(2), because rename replaces an
// existing target. If hard links are unsupported by the filesystem, the write
// fails closed (after removing its temp file) instead of weakening the
// no-overwrite rule.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".hrt-cold-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := file.Name()
	closed := false
	cleanup := func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(tempPath)
	}

	if err := file.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("set temp file mode: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		closed = true
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	if err := os.Link(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", errOutputExists, path)
		}
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("output was published but temp cleanup failed: %w", err)
	}
	if directoryHandle, err := os.Open(directory); err == nil {
		// Some platforms/filesystems do not support directory fsync. A
		// successful link is still visible atomically; only crash durability
		// is left unproven.
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

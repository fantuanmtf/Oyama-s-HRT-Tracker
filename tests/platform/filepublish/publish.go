// Package filepublish mirrors the Phase 1F atomic no-replace publish logic for
// cross-platform test execution. It is test infrastructure, not production app
// code, and it deliberately has no external dependencies.
package filepublish

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AtomicWrite writes data to a temporary file in the target directory, syncs
// it, and publishes it with a hard link. The link is an atomic no-replace
// operation on filesystems that support it: an existing target, including one
// created by a concurrent writer, is never overwritten.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	CleanupStaleTemps(dir, time.Hour)

	f, err := os.CreateTemp(dir, ".hrt-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = f.Close(); _ = os.Remove(tmp) }
	if err := f.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if err := os.Link(tmp, path); err != nil {
		_ = os.Remove(tmp)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("target exists: %w", err)
		}
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	if err := os.Remove(tmp); err != nil {
		return fmt.Errorf("remove published temp link: %w", err)
	}
	if d, err := os.Open(dir); err == nil {
		// Directory fsync is best-effort; Windows may not support it.
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// CleanupStaleTemps removes only this harness's own old temporary files. It
// skips non-regular files and anything newer than olderThan.
func CleanupStaleTemps(dir string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".hrt-tmp-") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

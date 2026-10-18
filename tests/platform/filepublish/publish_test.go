package filepublish

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNoReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "backup.hrtb")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := AtomicWrite(target, []byte("replacement"), 0o600)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected fs.ErrExist, got %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("existing target changed: %q", got)
	}
}

func TestConcurrentWritersExactlyOneWins(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "concurrent.hrtb")
	const writers = 8
	start := make(chan struct{})
	results := make(chan struct {
		data []byte
		err  error
	}, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		data := []byte(strings.Repeat(string(rune('A'+i)), 4096))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- struct {
				data []byte
				err  error
			}{data, AtomicWrite(target, data, 0o600)}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	wins := 0
	var winner []byte
	for r := range results {
		if r.err == nil {
			wins++
			winner = r.data
		} else if !errors.Is(r.err, fs.ErrExist) {
			t.Errorf("unexpected writer error: %v", r.err)
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner, got %d", wins)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, winner) {
		t.Fatal("target content does not match the winning writer")
	}
	assertNoTemps(t, dir)
}

func TestCleanupOnlyOwnOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".hrt-tmp-old")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	newTemp := filepath.Join(dir, ".hrt-tmp-new")
	if err := os.WriteFile(newTemp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	CleanupStaleTemps(dir, time.Hour)

	if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old own temp was not removed: %v", err)
	}
	for _, path := range []string{keep, newTemp} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated/new file removed: %s: %v", path, err)
		}
	}
}

func TestFailureLeavesNoOutputOrTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "backup.hrtb")
	err := AtomicWrite(target, []byte("data"), 0o600)
	if err == nil {
		t.Fatal("expected failure for missing parent directory")
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("failure left a target: %v", statErr)
	}
	assertNoTemps(t, dir)
}

func TestPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "permissions.hrtb")
	if err := AtomicWrite(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		// Windows does not expose Unix 0600 semantics through Go's FileMode.
		// The ACL is not verified here; CI must mark Windows ACL protection
		// NOT_TESTED until a native ACL assertion exists.
		t.Logf("Windows ACL not asserted; mode reported as %v", info.Mode().Perm())
		return
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected 0600, got %o", got)
	}
}

func TestUnicodeAndSpacePath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "unicodé space.hrtb")
	if err := AtomicWrite(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "data" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".hrt-tmp-") {
			t.Fatalf("leftover temp file: %s", entry.Name())
		}
	}
}

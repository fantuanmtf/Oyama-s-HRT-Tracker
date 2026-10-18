package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestAtomicWriteNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")
	if err := atomicWrite(target, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(target, []byte("two"), 0o600); !errors.Is(err, errOutputExists) {
		t.Fatalf("got %v, want output exists", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one" {
		t.Fatalf("existing target changed: %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %04o, want 0600", info.Mode().Perm())
		}
	}
	assertNoTempFiles(t, dir)
}

func TestAtomicWriteConcurrentNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	const writers = 8
	var wait sync.WaitGroup
	results := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wait.Add(1)
		go func(value byte) {
			defer wait.Done()
			results <- atomicWrite(target, []byte{value}, 0o600)
		}(byte('a' + i))
	}
	wait.Wait()
	close(results)

	successes := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errOutputExists):
		default:
			t.Fatalf("unexpected concurrent write error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one winner, got %d", successes)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0] < 'a' || data[0] >= 'a'+writers {
		t.Fatalf("unexpected target content: %q", data)
	}
	assertNoTempFiles(t, dir)
}

func TestAtomicWriteMissingParent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "out.json")
	if err := atomicWrite(target, []byte("data"), 0o600); err == nil {
		t.Fatal("expected failure when the parent directory is missing")
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected target after failure: %v", err)
	}
}

func TestAtomicWriteTargetIsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(target, []byte("data"), 0o600); err == nil {
		t.Fatal("expected failure when the target is a directory")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("target directory changed: %v", err)
	}
	assertNoTempFiles(t, dir)
}

func TestAtomicWriteReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL behavior is not equivalent to Unix mode bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission test is not meaningful as root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	target := filepath.Join(dir, "out.json")
	if err := atomicWrite(target, []byte("data"), 0o600); err == nil {
		t.Fatal("expected write failure in a read-only directory")
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected target after failure: %v", err)
	}
	assertNoTempFiles(t, dir)
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	temps, err := filepath.Glob(filepath.Join(dir, ".hrt-cold-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("leftover temp files: %v", temps)
	}
}

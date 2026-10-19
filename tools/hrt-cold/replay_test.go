package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplayStateRoundTripAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "replay.json")
	state := newReplayState()
	state.record("fingerprint-a", "2030-02-01T00:00:00Z", "hash-a")
	if err := writeReplayState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadReplayState(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := loaded.Signers["fingerprint-a"]
	if !ok {
		t.Fatal("signer entry missing after round trip")
	}
	if entry.LastCreatedAt != "2030-02-01T00:00:00Z" || entry.LastContentSHA256 != "hash-a" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("replay state mode = %04o, want 0600", info.Mode().Perm())
		}
	}
}

func TestCheckReplayStatuses(t *testing.T) {
	state := newReplayState()
	status, _, err := checkReplay(state, "signer", "2030-01-01T00:00:00Z", "hash-a", false)
	if err != nil || status != "new" {
		t.Fatalf("first check = %q, %v; want new", status, err)
	}
	state.record("signer", "2030-01-01T00:00:00Z", "hash-a")

	status, _, err = checkReplay(state, "signer", "2030-01-01T00:00:00Z", "hash-a", false)
	if err != nil || status != "repeat" {
		t.Fatalf("repeat check = %q, %v; want repeat", status, err)
	}

	if _, _, err := checkReplay(state, "signer", "2029-12-01T00:00:00Z", "hash-old", false); !errors.Is(
		err,
		errOlderBackup,
	) {
		t.Fatalf("older check error = %v, want errOlderBackup", err)
	}
	status, _, err = checkReplay(state, "signer", "2029-12-01T00:00:00Z", "hash-old", true)
	if err != nil || status != "older-allowed" {
		t.Fatalf("allowed older check = %q, %v; want older-allowed", status, err)
	}

	if _, _, err := checkReplay(state, "signer", "2030-01-01T00:00:00Z", "hash-b", false); err == nil {
		t.Fatal("same-time different content was accepted")
	}
	status, _, err = checkReplay(state, "signer", "2030-01-01T00:00:00Z", "hash-b", true)
	if err != nil || status != "conflict-allowed" {
		t.Fatalf("allowed conflict check = %q, %v; want conflict-allowed", status, err)
	}

	status, _, err = checkReplay(state, "signer", "2030-03-01T00:00:00Z", "hash-c", false)
	if err != nil || status != "newer" {
		t.Fatalf("newer check = %q, %v; want newer", status, err)
	}
}

func TestReplayStateRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplayState(corrupt); err == nil {
		t.Fatal("corrupt replay state was accepted")
	}

	versioned := filepath.Join(dir, "version.json")
	if err := os.WriteFile(versioned, []byte(`{"version":9,"signers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplayState(versioned); err == nil {
		t.Fatal("unknown replay state version was accepted")
	}

	if runtime.GOOS != "windows" {
		worldWritable := filepath.Join(dir, "world.json")
		if err := os.WriteFile(worldWritable, []byte(`{"version":1,"signers":{}}`), 0o666); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(worldWritable, 0o666); err != nil {
			t.Fatal(err)
		}
		if _, err := loadReplayState(worldWritable); err == nil {
			t.Fatal("group/world-writable replay state was accepted")
		}
	}
}

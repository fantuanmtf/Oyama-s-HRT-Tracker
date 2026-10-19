package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Replay protection is local and best-effort. It remembers the newest
// createdAt and content hash this device has accepted for each verified
// signer, so restoring an older signed backup is refused by default. It is
// not a global freshness service: a compromised signer key, a different
// device with no state, or deleting the state file all defeat it.

const replayStateVersion = 1

var errOlderBackup = errors.New("backup is older than the newest verified backup for this signer")

type replayEntry struct {
	LastCreatedAt     string `json:"lastCreatedAt"`
	LastContentSHA256 string `json:"lastContentSHA256"`
	LastSeenAt        string `json:"lastSeenAt"`
}

type replayState struct {
	Version int                    `json:"version"`
	Signers map[string]replayEntry `json:"signers"`
}

func newReplayState() *replayState {
	return &replayState{Version: replayStateVersion, Signers: make(map[string]replayEntry)}
}

func defaultReplayStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			if err != nil {
				return "", fmt.Errorf("locate user config directory: %w", err)
			}
			return "", errors.New("locate user config directory: empty path")
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "hrt-cold", "replay.json"), nil
}

func loadReplayState(path string) (*replayState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newReplayState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read replay state: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat replay state: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("replay state %s is writable by group or others", path)
	}
	var state replayState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse replay state: %w", err)
	}
	if state.Version != replayStateVersion {
		return nil, fmt.Errorf("unsupported replay state version %d", state.Version)
	}
	if state.Signers == nil {
		state.Signers = make(map[string]replayEntry)
	}
	return &state, nil
}

func parseReplayTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("replay state timestamp %q is not RFC 3339: %w", value, err)
	}
	return parsed, nil
}

// checkReplay compares a freshly verified bundle with the local state.
// It returns "new", "newer", "repeat", "older-allowed", or
// "conflict-allowed". Older and same-time-different-content results fail
// unless allowOlder is set; repeated decryption of the exact same bundle is
// allowed because it changes nothing.
func checkReplay(
	state *replayState,
	signer, createdAt, contentSHA256 string,
	allowOlder bool,
) (string, replayEntry, error) {
	entry, seen := state.Signers[signer]
	if !seen {
		return "new", entry, nil
	}
	last, err := parseReplayTime(entry.LastCreatedAt)
	if err != nil {
		return "", entry, err
	}
	incoming, err := parseReplayTime(createdAt)
	if err != nil {
		return "", entry, err
	}
	switch {
	case incoming.Before(last):
		if !allowOlder {
			return "", entry, fmt.Errorf(
				"%w: incoming %s is older than %s (use --allow-older to override)",
				errOlderBackup,
				createdAt,
				entry.LastCreatedAt,
			)
		}
		return "older-allowed", entry, nil
	case incoming.Equal(last):
		if contentSHA256 == entry.LastContentSHA256 {
			return "repeat", entry, nil
		}
		if !allowOlder {
			return "", entry, fmt.Errorf(
				"%w: %s was seen before with different content (use --allow-older to override)",
				errOlderBackup,
				createdAt,
			)
		}
		return "conflict-allowed", entry, nil
	default:
		return "newer", entry, nil
	}
}

// record raises the high-water mark only for a bundle that is newer than (or
// the first seen for) this signer. An allowed older replay must not lower it.
func (s *replayState) record(signer, createdAt, contentSHA256 string) {
	s.Signers[signer] = replayEntry{
		LastCreatedAt:     createdAt,
		LastContentSHA256: contentSHA256,
		LastSeenAt:        time.Now().UTC().Format(time.RFC3339),
	}
}

func writeReplayState(path string, state *replayState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create replay state directory: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay state: %w", err)
	}
	raw = append(raw, '\n')

	temp, err := os.CreateTemp(dir, ".hrt-cold-replay-*")
	if err != nil {
		return fmt.Errorf("create replay state temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set replay state mode: %w", err)
	}
	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		return fmt.Errorf("write replay state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync replay state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close replay state: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		// Windows does not replace an existing file with os.Rename. The state
		// is not secret and can be replaced atomically on Unix; on Windows a
		// short gap is preferable to failing every second decrypt.
		if runtime.GOOS != "windows" {
			return fmt.Errorf("publish replay state: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace replay state: %w", removeErr)
		}
		if renameErr := os.Rename(tempName, path); renameErr != nil {
			return fmt.Errorf("publish replay state: %w", renameErr)
		}
	}
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

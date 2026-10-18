package filepublish

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a normal test. The parent test re-executes the test
// binary with GO_WANT_HELPER_PROCESS=1 so the writers are separate processes,
// not goroutines in one address space.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	sep := -1
	for i, arg := range os.Args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || len(os.Args) < sep+4 {
		os.Exit(3)
	}
	target, payload, ready, start := os.Args[sep+1], os.Args[sep+2], os.Args[sep+3], os.Args[sep+4]
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		os.Exit(3)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(start); err == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(3)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := AtomicWrite(target, []byte(payload), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestCrossProcessWritersExactlyOneWins(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cross-process.hrtb")
	start := filepath.Join(dir, "start")
	const writers = 4

	type process struct {
		cmd     *exec.Cmd
		ready   string
		payload string
	}
	procs := make([]process, 0, writers)
	for i := 0; i < writers; i++ {
		ready := filepath.Join(dir, fmt.Sprintf("ready-%d", i))
		payload := fmt.Sprintf("writer-%d", i)
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", target, payload, ready, start)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %d: %v", i, err)
		}
		procs = append(procs, process{cmd: cmd, ready: ready, payload: payload})
	}

	deadline := time.Now().Add(15 * time.Second)
	for _, p := range procs {
		for {
			if _, err := os.Stat(p.ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for helper readiness")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := os.WriteFile(start, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}

	successes := 0
	payloads := make(map[string]bool)
	for _, p := range procs {
		payloads[p.payload] = true
		err := p.cmd.Wait()
		if err == nil {
			successes++
			continue
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			t.Errorf("helper failed unexpectedly: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one winning process, got %d", successes)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !payloads[string(got)] {
		t.Fatalf("target has unexpected content %q", got)
	}
	assertNoTemps(t, dir)
}

// TestWindowsACLSmoke records the ACL of a published file. It is a smoke
// check, not proof that Windows ACLs provide Unix-0600-equivalent protection.
func TestWindowsACLSmoke(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows ACL check")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "acl.hrtb")
	if err := AtomicWrite(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("icacls", target).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls failed: %v\n%s", err, out)
	}
	t.Logf("icacls %s:\n%s", target, out)
	for _, broad := range []string{"Everyone:(F)", "Everyone:(M)", "BUILTIN\\Users:(F)", "BUILTIN\\Users:(M)"} {
		if bytes.Contains(out, []byte(broad)) {
			t.Errorf("ACL grants broad access %q", broad)
		}
	}
	if !bytes.Contains(out, []byte(":")) {
		t.Fatalf("unexpected icacls output: %q", strings.TrimSpace(string(out)))
	}
}

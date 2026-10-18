package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

type testIdentity struct {
	privatePath    string
	publicPath     string
	passphrasePath string
	fingerprint    string
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuffer, errBuffer bytes.Buffer
	code = run(args, &outBuffer, &errBuffer)
	return outBuffer.String(), errBuffer.String(), code
}

func newTestIdentity(t *testing.T, dir string) *testIdentity {
	t.Helper()
	return newTestIdentityWithPassphrase(t, dir, "synthetic test passphrase")
}

func newTestIdentityWithPassphrase(t *testing.T, dir, passphrase string) *testIdentity {
	t.Helper()
	passphrasePath := writeTestFile(t, filepath.Join(dir, "passphrase.txt"), passphrase+"\n", 0o600)
	privatePath := filepath.Join(dir, "private.asc")
	publicPath := filepath.Join(dir, "public.asc")
	stdout, stderr, code := runCLI(t,
		"keygen",
		"--private-out", privatePath,
		"--public-out", publicPath,
		"--passphrase-file", passphrasePath,
	)
	if code != 0 {
		t.Fatalf("keygen failed with code %d: %s", code, stderr)
	}
	return &testIdentity{
		privatePath:    privatePath,
		publicPath:     publicPath,
		passphrasePath: passphrasePath,
		fingerprint:    outputValue(t, stdout, "fingerprint"),
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readSyntheticBundle(t *testing.T) []byte {
	t.Helper()
	return readTestFile(t, filepath.Join("testdata", "synthetic-bundle.json"))
}

func outputValue(t *testing.T, output, key string) string {
	t.Helper()
	prefix := key + ":"
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("output does not contain %q: %s", key, output)
	return ""
}

func requireMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got %v", path, err)
	}
}

func requireHexFingerprint(t *testing.T, fingerprint string) {
	t.Helper()
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil {
		t.Fatalf("fingerprint is not hex: %q", fingerprint)
	}
	if len(decoded) < 20 {
		t.Fatalf("fingerprint is too short: %q", fingerprint)
	}
}

func writeEncryptedContainer(t *testing.T, identity *testIdentity, plaintext []byte, path string) {
	t.Helper()
	publicArmor := string(readTestFile(t, identity.publicPath))
	containerBytes, _, err := encryptBundle(publicArmor, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, containerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func encryptSyntheticBundle(t *testing.T, identity *testIdentity, dir string) (bundlePath, containerPath string) {
	t.Helper()
	bundlePath = filepath.Join(dir, "bundle.json")
	containerPath = filepath.Join(dir, "backup.hrtb")
	if err := os.WriteFile(bundlePath, readSyntheticBundle(t), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t,
		"encrypt",
		"--recipient", identity.publicPath,
		"--in", bundlePath,
		"--out", containerPath,
	)
	if code != 0 {
		t.Fatalf("encrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "recipientFingerprint"); got != identity.fingerprint {
		t.Fatalf("recipient fingerprint = %s, want %s", got, identity.fingerprint)
	}
	return bundlePath, containerPath
}

func TestKeygenCreatesUsableKeysAndDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	requireHexFingerprint(t, identity.fingerprint)

	privateArmor := readTestFile(t, identity.privatePath)
	publicArmor := readTestFile(t, identity.publicPath)

	privateKey, err := crypto.NewPrivateKeyFromArmored(string(privateArmor), []byte("synthetic test passphrase"))
	if err != nil {
		t.Fatalf("correct passphrase rejected: %v", err)
	}
	defer privateKey.ClearPrivateParams()
	if got := privateKey.GetFingerprint(); got != identity.fingerprint {
		t.Fatalf("private fingerprint = %s, want %s", got, identity.fingerprint)
	}
	if _, err := crypto.NewPrivateKeyFromArmored(string(privateArmor), []byte("wrong passphrase")); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	publicKey, err := crypto.NewKeyFromArmored(string(publicArmor))
	if err != nil {
		t.Fatal(err)
	}
	if got := publicKey.GetFingerprint(); got != identity.fingerprint {
		t.Fatalf("public fingerprint = %s, want %s", got, identity.fingerprint)
	}

	if runtime.GOOS != "windows" {
		privateInfo, err := os.Stat(identity.privatePath)
		if err != nil {
			t.Fatal(err)
		}
		if privateInfo.Mode().Perm() != 0o600 {
			t.Fatalf("private key mode = %04o, want 0600", privateInfo.Mode().Perm())
		}
		publicInfo, err := os.Stat(identity.publicPath)
		if err != nil {
			t.Fatal(err)
		}
		if publicInfo.Mode().Perm() != 0o644 {
			t.Fatalf("public key mode = %04o, want 0644", publicInfo.Mode().Perm())
		}
	}

	beforePrivate := readTestFile(t, identity.privatePath)
	beforePublic := readTestFile(t, identity.publicPath)
	if _, stderr, code := runCLI(t,
		"keygen",
		"--private-out", identity.privatePath,
		"--public-out", identity.publicPath,
		"--passphrase-file", identity.passphrasePath,
	); code == 0 {
		t.Fatalf("keygen overwrote existing outputs: %s", stderr)
	}
	if !bytes.Equal(beforePrivate, readTestFile(t, identity.privatePath)) {
		t.Fatal("existing private key changed")
	}
	if !bytes.Equal(beforePublic, readTestFile(t, identity.publicPath)) {
		t.Fatal("existing public key changed")
	}
	assertNoTempFiles(t, dir)
}

func TestKeygenRejectsUnsafePassphraseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses NTFS ACLs; Unix mode bits are not meaningful")
	}
	dir := t.TempDir()
	passphrasePath := writeTestFile(t, filepath.Join(dir, "passphrase.txt"), "synthetic\n", 0o644)
	privatePath := filepath.Join(dir, "private.asc")
	publicPath := filepath.Join(dir, "public.asc")
	_, stderr, code := runCLI(t,
		"keygen",
		"--private-out", privatePath,
		"--public-out", publicPath,
		"--passphrase-file", passphrasePath,
	)
	if code == 0 {
		t.Fatal("keygen accepted a group/world-readable passphrase file")
	}
	if !strings.Contains(stderr, "not private enough") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireMissing(t, privatePath)
	requireMissing(t, publicPath)
	assertNoTempFiles(t, dir)
}

func TestKeygenRejectsSameOutputPath(t *testing.T) {
	dir := t.TempDir()
	passphrasePath := writeTestFile(t, filepath.Join(dir, "passphrase.txt"), "synthetic\n", 0o600)
	samePath := filepath.Join(dir, "same.asc")
	_, stderr, code := runCLI(t,
		"keygen",
		"--private-out", samePath,
		"--public-out", samePath,
		"--passphrase-file", passphrasePath,
	)
	if code == 0 {
		t.Fatal("keygen accepted the same output path for both keys")
	}
	if !strings.Contains(stderr, "must be different") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireMissing(t, samePath)
	assertNoTempFiles(t, dir)
}

func TestEncryptDecryptRoundTripAndInspect(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	bundlePath, containerPath := encryptSyntheticBundle(t, identity, dir)
	decryptedPath := filepath.Join(dir, "decrypted.json")

	stdout, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", decryptedPath,
	)
	if code != 0 {
		t.Fatalf("decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "privateKeyFingerprint"); got != identity.fingerprint {
		t.Fatalf("private fingerprint = %s, want %s", got, identity.fingerprint)
	}
	if !bytes.Equal(readTestFile(t, bundlePath), readTestFile(t, decryptedPath)) {
		t.Fatal("round-trip bytes differ from the input bundle")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(decryptedPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("decrypted output mode = %04o, want 0600", info.Mode().Perm())
		}
	}

	containerBytes := readTestFile(t, containerPath)
	parsed, err := parseContainer(containerBytes)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Version != containerVersion || parsed.Flags != containerFlags {
		t.Fatalf("unexpected container header: version=%d flags=%d", parsed.Version, parsed.Flags)
	}
	sum := sha256.Sum256(parsed.Payload)

	stdout, stderr, code = runCLI(t,
		"inspect",
		"--in", containerPath,
		"--recipient", identity.publicPath,
	)
	if code != 0 {
		t.Fatalf("inspect failed with code %d: %s", code, stderr)
	}
	if outputValue(t, stdout, "magic") != "HRTB" || outputValue(t, stdout, "containerVersion") != "1" {
		t.Fatalf("unexpected inspect metadata: %s", stdout)
	}
	if outputValue(t, stdout, "payloadBytes") != itoa(len(parsed.Payload)) {
		t.Fatalf("unexpected payload size: %s", stdout)
	}
	if outputValue(t, stdout, "payloadSHA256") != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected payload hash: %s", stdout)
	}
	if outputValue(t, stdout, "recipientCount") != "1" {
		t.Fatalf("unexpected recipient count: %s", stdout)
	}
	if outputValue(t, stdout, "primaryFingerprint") != identity.fingerprint {
		t.Fatalf("unexpected primary fingerprint: %s", stdout)
	}
	requireHexFingerprint(t, outputValue(t, stdout, "recipientFingerprint"))
	if outputValue(t, stdout, "decrypted") != "false" {
		t.Fatalf("inspect must not claim decryption: %s", stdout)
	}

	stdout, stderr, code = runCLI(t, "inspect", "--in", containerPath)
	if code != 0 {
		t.Fatalf("inspect without recipient failed with code %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "recipientKeyID:") {
		t.Fatalf("inspect did not show a recipient key ID: %s", stdout)
	}
	if strings.Contains(stdout, "primaryFingerprint:") || strings.Contains(stdout, "recipientFingerprint:") {
		t.Fatalf("inspect claimed a fingerprint without a public key: %s", stdout)
	}
	assertNoTempFiles(t, dir)
}

func TestInspectRejectsWrongRecipient(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	otherDir := t.TempDir()
	other := newTestIdentity(t, otherDir)

	_, stderr, code := runCLI(t,
		"inspect",
		"--in", containerPath,
		"--recipient", other.publicPath,
	)
	if code == 0 {
		t.Fatal("inspect accepted a non-recipient public key")
	}
	if !strings.Contains(stderr, "not encrypted to the supplied recipient key") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	otherDir := t.TempDir()
	other := newTestIdentityWithPassphrase(t, otherDir, "other synthetic passphrase")
	outputPath := filepath.Join(dir, "wrong-key.json")

	_, stderr, code := runCLI(t,
		"decrypt",
		"--private", other.privatePath,
		"--passphrase-file", other.passphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt accepted the wrong private key")
	}
	requireMissing(t, outputPath)
	assertNoTempFiles(t, dir)
	if strings.Contains(stderr, "other synthetic passphrase") {
		t.Fatal("error output leaked the passphrase")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	wrongPassphrasePath := writeTestFile(t, filepath.Join(dir, "wrong-passphrase.txt"), "wrong synthetic passphrase\n", 0o600)
	outputPath := filepath.Join(dir, "wrong-passphrase.json")

	_, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", wrongPassphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt accepted the wrong passphrase")
	}
	requireMissing(t, outputPath)
	assertNoTempFiles(t, dir)
	if strings.Contains(stderr, "wrong synthetic passphrase") {
		t.Fatal("error output leaked the passphrase")
	}
}

func TestDecryptRejectsTamperTruncationAndContainerChanges(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	original := readTestFile(t, containerPath)

	mutate := func(name string, mutate func([]byte) []byte) {
		t.Helper()
		raw := mutate(append([]byte(nil), original...))
		path := filepath.Join(dir, name+".hrtb")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		outputPath := filepath.Join(dir, name+".json")
		_, _, code := runCLI(t,
			"decrypt",
			"--private", identity.privatePath,
			"--passphrase-file", identity.passphrasePath,
			"--in", path,
			"--out", outputPath,
		)
		if code == 0 {
			t.Fatalf("%s: decrypt accepted an invalid container", name)
		}
		requireMissing(t, outputPath)
		assertNoTempFiles(t, dir)
	}

	mutate("tampered", func(raw []byte) []byte {
		raw[len(raw)-1] ^= 0xff
		return raw
	})
	mutate("truncated", func(raw []byte) []byte { return raw[:len(raw)-1] })
	mutate("bad-magic", func(raw []byte) []byte {
		copy(raw[:4], "XXXX")
		return raw
	})
	mutate("unknown-version", func(raw []byte) []byte {
		binary.BigEndian.PutUint16(raw[4:6], containerVersion+1)
		return raw
	})
	mutate("reserved-flags", func(raw []byte) []byte {
		binary.BigEndian.PutUint16(raw[6:8], 1)
		return raw
	})
	mutate("trailing", func(raw []byte) []byte { return append(raw, 0) })

	tamperedPath := filepath.Join(dir, "tampered.hrtb")
	if _, _, code := runCLI(t, "inspect", "--in", tamperedPath); code != 0 {
		t.Fatal("inspect unexpectedly failed on intact container metadata")
	}
	for _, name := range []string{"truncated", "bad-magic", "unknown-version", "reserved-flags", "trailing"} {
		if _, _, code := runCLI(t, "inspect", "--in", filepath.Join(dir, name+".hrtb")); code == 0 {
			t.Fatalf("inspect accepted invalid container %s", name)
		}
	}
}

func TestExistingOutputIsNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	bundlePath, containerPath := encryptSyntheticBundle(t, identity, dir)

	decryptOutput := filepath.Join(dir, "decrypt-existing.json")
	writeTestFile(t, decryptOutput, "sentinel-decrypt", 0o600)
	if _, _, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", decryptOutput,
	); code == 0 {
		t.Fatal("decrypt overwrote an existing output")
	}
	if string(readTestFile(t, decryptOutput)) != "sentinel-decrypt" {
		t.Fatal("existing decrypt output changed")
	}

	encryptOutput := filepath.Join(dir, "encrypt-existing.hrtb")
	writeTestFile(t, encryptOutput, "sentinel-encrypt", 0o600)
	if _, _, code := runCLI(t,
		"encrypt",
		"--recipient", identity.publicPath,
		"--in", bundlePath,
		"--out", encryptOutput,
	); code == 0 {
		t.Fatal("encrypt overwrote an existing output")
	}
	if string(readTestFile(t, encryptOutput)) != "sentinel-encrypt" {
		t.Fatal("existing encrypt output changed")
	}
	assertNoTempFiles(t, dir)
}

func TestOversizedInputsFailClosed(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	oversizedBundle := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(oversizedBundle, bytes.Repeat([]byte(" "), maxBundleBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	bundleOutput := filepath.Join(dir, "oversized.hrtb")
	if _, _, code := runCLI(t,
		"encrypt",
		"--recipient", identity.publicPath,
		"--in", oversizedBundle,
		"--out", bundleOutput,
	); code == 0 {
		t.Fatal("encrypt accepted an oversized bundle")
	}
	requireMissing(t, bundleOutput)

	oversizedContainer := filepath.Join(dir, "oversized.hrtb.input")
	if err := os.WriteFile(oversizedContainer, bytes.Repeat([]byte{0}, maxContainerBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	containerOutput := filepath.Join(dir, "oversized.json.out")
	if _, _, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", oversizedContainer,
		"--out", containerOutput,
	); code == 0 {
		t.Fatal("decrypt accepted an oversized container")
	}
	requireMissing(t, containerOutput)
	assertNoTempFiles(t, dir)
}

func TestDecryptValidatesPlaintextBundle(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	publicArmor := string(readTestFile(t, identity.publicPath))
	invalidBundles := map[string]string{
		"wrong-format":  `{"format":"other","formatVersion":1,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem","modes":{"transfem":{},"transmasc":{}}}`,
		"wrong-version": `{"format":"oyama-hrt-complete-backup","formatVersion":2,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem","modes":{"transfem":{},"transmasc":{}}}`,
		"not-json":      `not-json`,
		"duplicate-key": `{"format":"oyama-hrt-complete-backup","format":"oyama-hrt-complete-backup","formatVersion":1,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem","modes":{"transfem":{},"transmasc":{}}}`,
	}
	for name, bundle := range invalidBundles {
		t.Run(name, func(t *testing.T) {
			containerBytes, _, err := encryptBundle(publicArmor, []byte(bundle))
			if err != nil {
				t.Fatal(err)
			}
			containerPath := filepath.Join(dir, name+".hrtb")
			if err := os.WriteFile(containerPath, containerBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(dir, name+".json")
			_, _, code := runCLI(t,
				"decrypt",
				"--private", identity.privatePath,
				"--passphrase-file", identity.passphrasePath,
				"--in", containerPath,
				"--out", outputPath,
			)
			if code == 0 {
				t.Fatal("decrypt accepted a validly encrypted but invalid bundle")
			}
			requireMissing(t, outputPath)
			assertNoTempFiles(t, dir)
		})
	}
}

func TestWriteFailureLeavesNoPartialOutput(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	missingParent := filepath.Join(dir, "missing-parent")
	outputPath := filepath.Join(missingParent, "out.json")
	_, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt succeeded with a missing output parent")
	}
	if !strings.Contains(stderr, "create temp file") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
	requireMissing(t, outputPath)
	requireMissing(t, missingParent)
	assertNoTempFiles(t, dir)
}

func TestCLIUsage(t *testing.T) {
	if _, _, code := runCLI(t); code != 2 {
		t.Fatalf("no arguments: code = %d, want 2", code)
	}
	if _, _, code := runCLI(t, "unknown-command"); code != 2 {
		t.Fatalf("unknown command: code = %d, want 2", code)
	}
	if _, _, code := runCLI(t, "keygen"); code != 1 {
		t.Fatalf("keygen without flags: code = %d, want 1", code)
	}
	if _, _, code := runCLI(t, "encrypt"); code != 1 {
		t.Fatalf("encrypt without flags: code = %d, want 1", code)
	}
	if _, _, code := runCLI(t, "decrypt"); code != 1 {
		t.Fatalf("decrypt without flags: code = %d, want 1", code)
	}
	if _, _, code := runCLI(t, "inspect"); code != 1 {
		t.Fatalf("inspect without flags: code = %d, want 1", code)
	}
	if _, _, code := runCLI(t, "help"); code != 0 {
		t.Fatalf("help: code = %d, want 0", code)
	}
	if _, _, code := runCLI(t, "keygen", "--help"); code != 0 {
		t.Fatalf("keygen --help: code = %d, want 0", code)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

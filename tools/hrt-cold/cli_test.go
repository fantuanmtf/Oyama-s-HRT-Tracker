package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
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

func requireHexKeyID(t *testing.T, keyID string) {
	t.Helper()
	decoded, err := hex.DecodeString(keyID)
	if err != nil {
		t.Fatalf("key ID is not hex: %q", keyID)
	}
	if len(decoded) != 8 {
		t.Fatalf("key ID is not 8 bytes: %q", keyID)
	}
}

func writeEncryptedContainer(t *testing.T, identity *testIdentity, plaintext []byte, path string) {
	t.Helper()
	publicArmor := string(readTestFile(t, identity.publicPath))
	containerBytes, _, _, _, err := encryptBundle(publicArmor, "", nil, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, containerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func literalPayload(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := packet.SerializeLiteral(nopWriteCloser{&buffer}, true, "bundle.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(readSyntheticBundle(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func compressedPlaintextPayload(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed, err := packet.SerializeCompressed(nopWriteCloser{&buffer}, packet.CompressionZIP, nil)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := packet.SerializeLiteral(compressed, true, "bundle.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(readSyntheticBundle(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func signedOnlyPayload(t *testing.T, identity *testIdentity) []byte {
	t.Helper()
	passphrase, err := readPassphrase(identity.passphrasePath)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := crypto.NewPrivateKeyFromArmored(
		string(readTestFile(t, identity.privatePath)),
		passphrase,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer privateKey.ClearPrivateParams()
	var buffer bytes.Buffer
	writer, err := openpgp.Sign(&buffer, openpgp.EntityList{privateKey.GetEntity()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(readSyntheticBundle(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func encryptionKeyID(t *testing.T, identity *testIdentity) uint64 {
	t.Helper()
	publicKey, err := crypto.NewKeyFromArmored(string(readTestFile(t, identity.publicPath)))
	if err != nil {
		t.Fatal(err)
	}
	encryptionKey, ok := publicKey.GetEntity().EncryptionKey(time.Now(), nil)
	if !ok {
		t.Fatal("supplied key has no encryption key")
	}
	return encryptionKey.PublicKey.KeyId
}

func writeContainerPayload(t *testing.T, payload []byte, path string) {
	t.Helper()
	containerBytes, err := makeContainer(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, containerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

func firstPKESKPayloadParts(t *testing.T, containerPath string) (pkesk, remainder []byte) {
	t.Helper()
	parsed, err := parseContainer(readTestFile(t, containerPath))
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(parsed.Payload)
	if _, err := packet.Read(reader); err != nil {
		t.Fatal(err)
	}
	consumed := len(parsed.Payload) - reader.Len()
	return parsed.Payload[:consumed], parsed.Payload[consumed:]
}

func rewriteFirstPKESKRecipient(t *testing.T, containerPath string, identity *testIdentity) []byte {
	t.Helper()
	parsed, err := parseContainer(readTestFile(t, containerPath))
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(parsed.Payload)
	first, err := packet.Read(reader)
	if err != nil {
		t.Fatal(err)
	}
	encryptedKey, ok := first.(*packet.EncryptedKey)
	if !ok {
		t.Fatalf("first packet is %T, want *packet.EncryptedKey", first)
	}
	consumed := len(parsed.Payload) - reader.Len()

	publicKey, err := crypto.NewKeyFromArmored(string(readTestFile(t, identity.publicPath)))
	if err != nil {
		t.Fatal(err)
	}
	encryptionKey, ok := publicKey.GetEntity().EncryptionKey(time.Now(), nil)
	if !ok {
		t.Fatal("supplied key has no encryption key")
	}

	encryptedKey.KeyId = encryptionKey.PublicKey.KeyId
	if encryptedKey.Version == 6 && encryptedKey.KeyVersion != 0 {
		encryptedKey.KeyVersion = encryptionKey.PublicKey.Version
		encryptedKey.KeyFingerprint = append([]byte(nil), encryptionKey.PublicKey.Fingerprint...)
	}
	var rewritten bytes.Buffer
	if err := encryptedKey.Serialize(&rewritten); err != nil {
		t.Fatal(err)
	}
	payload := append(rewritten.Bytes(), parsed.Payload[consumed:]...)
	containerBytes, err := makeContainer(payload)
	if err != nil {
		t.Fatal(err)
	}
	return containerBytes
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
		"--signer-private", identity.privatePath,
		"--signer-passphrase-file", identity.passphrasePath,
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

// encryptBundleText writes a synthetic bundle with the given text and encrypts
// it with the identity's signing key.
func encryptBundleText(
	t *testing.T,
	identity *testIdentity,
	dir, name, bundle string,
) (bundlePath, containerPath string) {
	t.Helper()
	bundlePath = filepath.Join(dir, name+".json")
	containerPath = filepath.Join(dir, name+".hrtb")
	if err := os.WriteFile(bundlePath, []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t,
		"encrypt",
		"--recipient", identity.publicPath,
		"--signer-private", identity.privatePath,
		"--signer-passphrase-file", identity.passphrasePath,
		"--in", bundlePath,
		"--out", containerPath,
	)
	if code != 0 {
		t.Fatalf("encrypt failed with code %d: %s", code, stderr)
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
	requireHexKeyID(t, outputValue(t, stdout, "advertisedRecipientKeyID"))
	if outputValue(t, stdout, "suppliedRecipientFingerprint") != identity.fingerprint {
		t.Fatalf("unexpected supplied fingerprint: %s", stdout)
	}
	if outputValue(t, stdout, "advertisedRecipientMatchesSupplied") != "true" {
		t.Fatalf("supplied recipient did not match advertised key ID: %s", stdout)
	}
	requireHexFingerprint(t, outputValue(t, stdout, "advertisedRecipientMatchFingerprint"))
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect must not claim recipient verification: %s", stdout)
	}
	if outputValue(t, stdout, "decrypted") != "false" {
		t.Fatalf("inspect must not claim decryption: %s", stdout)
	}

	stdout, stderr, code = runCLI(t, "inspect", "--in", containerPath)
	if code != 0 {
		t.Fatalf("inspect without recipient failed with code %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "advertisedRecipientKeyID:") {
		t.Fatalf("inspect did not show an advertised recipient key ID: %s", stdout)
	}
	if strings.Contains(stdout, "suppliedRecipientFingerprint:") ||
		strings.Contains(stdout, "advertisedRecipientMatchesSupplied:") ||
		strings.Contains(stdout, "recipientFingerprint:") {
		t.Fatalf("inspect claimed a supplied fingerprint without a public key: %s", stdout)
	}
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect must not claim recipient verification: %s", stdout)
	}
	assertNoTempFiles(t, dir)
}

func TestInspectReportsWrongSuppliedRecipientAsUnverified(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	otherDir := t.TempDir()
	other := newTestIdentity(t, otherDir)

	stdout, stderr, code := runCLI(t,
		"inspect",
		"--in", containerPath,
		"--recipient", other.publicPath,
	)
	if code != 0 {
		t.Fatalf("inspect treated an informational recipient mismatch as an error: %d: %s", code, stderr)
	}
	if outputValue(t, stdout, "advertisedRecipientMatchesSupplied") != "false" {
		t.Fatalf("inspect reported a match for a non-recipient key: %s", stdout)
	}
	if outputValue(t, stdout, "suppliedRecipientFingerprint") != other.fingerprint {
		t.Fatalf("unexpected supplied fingerprint: %s", stdout)
	}
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect must not claim recipient verification: %s", stdout)
	}
}

func TestInspectDoesNotVerifyForgedRecipientKeyID(t *testing.T) {
	dir := t.TempDir()
	attacker := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, attacker, dir)
	victimDir := t.TempDir()
	victim := newTestIdentity(t, victimDir)

	forgedPath := filepath.Join(dir, "forged-recipient.hrtb")
	if err := os.WriteFile(forgedPath, rewriteFirstPKESKRecipient(t, containerPath, victim), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"inspect",
		"--in", forgedPath,
		"--recipient", victim.publicPath,
	)
	if code != 0 {
		t.Fatalf("inspect failed to read forged PKESK metadata: %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "advertisedRecipientKeyID"); got != fmt.Sprintf("%016x", encryptionKeyID(t, victim)) {
		t.Fatalf("advertised recipient key ID = %s, want victim encryption key", got)
	}
	if outputValue(t, stdout, "advertisedRecipientMatchesSupplied") != "true" {
		t.Fatalf("forged PKESK key ID did not match supplied key: %s", stdout)
	}
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect claimed that an unauthenticated PKESK was verified: %s", stdout)
	}

	outputPath := filepath.Join(dir, "forged-recipient.json")
	_, stderr, code = runCLI(t,
		"decrypt",
		"--private", victim.privatePath,
		"--passphrase-file", victim.passphrasePath,
		"--in", forgedPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt accepted a container with a forged PKESK key ID")
	}
	if !strings.Contains(stderr, "incorrect key") && !strings.Contains(stderr, "wrong key") {
		t.Fatalf("unexpected forged-recipient decrypt error: %s", stderr)
	}
	requireMissing(t, outputPath)
	assertNoTempFiles(t, dir)
}

func TestInspectRejectsMissingRecipientOrEncryptedPayload(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)
	pkesk, encryptedPayload := firstPKESKPayloadParts(t, containerPath)

	tests := []struct {
		name    string
		payload []byte
		want    string
	}{
		{name: "no-recipient", payload: encryptedPayload, want: "no public-key recipient"},
		{name: "no-encrypted-payload", payload: pkesk, want: "no encrypted data packet"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, test.name+".hrtb")
			writeContainerPayload(t, test.payload, path)

			_, stderr, code := runCLI(t, "inspect", "--in", path)
			if code == 0 {
				t.Fatal("inspect accepted a container without a complete encrypted message")
			}
			if !strings.Contains(stderr, test.want) {
				t.Fatalf("unexpected inspect error: %s", stderr)
			}

			outputPath := filepath.Join(dir, test.name+".json")
			_, _, code = runCLI(t,
				"decrypt",
				"--private", identity.privatePath,
				"--passphrase-file", identity.passphrasePath,
				"--in", path,
				"--out", outputPath,
			)
			if code == 0 {
				t.Fatal("decrypt accepted a container without a complete encrypted message")
			}
			requireMissing(t, outputPath)
			assertNoTempFiles(t, dir)
		})
	}
}

func TestDecryptRejectsUnencryptedOpenPGPMessages(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	tests := []struct {
		name    string
		payload func(*testing.T) []byte
	}{
		{name: "literal", payload: literalPayload},
		{name: "compressed-plaintext", payload: compressedPlaintextPayload},
		{name: "signed-only", payload: func(t *testing.T) []byte { return signedOnlyPayload(t, identity) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, test.name+".hrtb")
			writeContainerPayload(t, test.payload(t), path)

			outputPath := filepath.Join(dir, test.name+".json")
			_, stderr, code := runCLI(t,
				"decrypt",
				"--private", identity.privatePath,
				"--passphrase-file", identity.passphrasePath,
				"--in", path,
				"--out", outputPath,
			)
			if code == 0 {
				t.Fatal("decrypt accepted a plaintext or signed-only OpenPGP payload")
			}
			if !strings.Contains(stderr, "not an encrypted OpenPGP message") {
				t.Fatalf("unexpected decrypt error: %s", stderr)
			}
			requireMissing(t, outputPath)

			_, stderr, code = runCLI(t, "inspect", "--in", path)
			if code == 0 {
				t.Fatal("inspect accepted a plaintext or signed-only OpenPGP payload")
			}
			if !strings.Contains(stderr, "not an encrypted OpenPGP message") {
				t.Fatalf("unexpected inspect error: %s", stderr)
			}
			assertNoTempFiles(t, dir)
		})
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
			containerBytes, _, _, _, err := encryptBundle(publicArmor, "", nil, []byte(bundle))
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

func TestVersionCommand(t *testing.T) {
	stdout, stderr, code := runCLI(t, "version")
	if code != 0 {
		t.Fatalf("version failed with code %d: %s", code, stderr)
	}
	for _, want := range []string{"hrt-cold " + buildVersion, "go: ", "platform: " + runtime.GOOS + "/" + runtime.GOARCH} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("version output missing %q: %s", want, stdout)
		}
	}
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

func TestSignedDecryptVerifiesProvenanceAndIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	bundlePath, containerPath := encryptSyntheticBundle(t, identity, dir)
	replayPath := filepath.Join(dir, "state", "replay.json")
	firstOut := filepath.Join(dir, "verified-first.json")

	stdout, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", replayPath,
		"--in", containerPath,
		"--out", firstOut,
	)
	if code != 0 {
		t.Fatalf("verified decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "signatureVerified"); got != "true" {
		t.Fatalf("signatureVerified = %s, want true", got)
	}
	if got := outputValue(t, stdout, "signerPrimaryFingerprint"); got != identity.fingerprint {
		t.Fatalf("signer primary fingerprint = %s, want %s", got, identity.fingerprint)
	}
	hash := sha256.Sum256(readTestFile(t, bundlePath))
	if got := outputValue(t, stdout, "contentSha256"); got != hex.EncodeToString(hash[:]) {
		t.Fatalf("content hash = %s, want %s", got, hex.EncodeToString(hash[:]))
	}
	if got := outputValue(t, stdout, "replayStatus"); got != "new" {
		t.Fatalf("first replay status = %s, want new", got)
	}
	if !bytes.Equal(readTestFile(t, bundlePath), readTestFile(t, firstOut)) {
		t.Fatal("verified round-trip bytes differ")
	}

	// Decrypting the same verified backup again is a no-op replay.
	secondOut := filepath.Join(dir, "verified-second.json")
	stdout, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", replayPath,
		"--in", containerPath,
		"--out", secondOut,
	)
	if code != 0 {
		t.Fatalf("repeat decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "replayStatus"); got != "repeat" {
		t.Fatalf("repeat replay status = %s, want repeat", got)
	}
}

func TestSeparateSigningKeyIsVerified(t *testing.T) {
	dir := t.TempDir()
	recipientDir := filepath.Join(dir, "recipient")
	signerDir := filepath.Join(dir, "signer")
	if err := os.MkdirAll(recipientDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(signerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	recipient := newTestIdentity(t, recipientDir)
	signer := newTestIdentity(t, signerDir)
	bundlePath := filepath.Join(dir, "bundle.json")
	containerPath := filepath.Join(dir, "backup.hrtb")
	if err := os.WriteFile(bundlePath, readSyntheticBundle(t), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t,
		"encrypt",
		"--recipient", recipient.publicPath,
		"--signer-private", signer.privatePath,
		"--signer-passphrase-file", signer.passphrasePath,
		"--in", bundlePath,
		"--out", containerPath,
	)
	if code != 0 {
		t.Fatalf("encrypt with separate signer failed with code %d: %s", code, stderr)
	}

	outPath := filepath.Join(dir, "decrypted.json")
	stdout, stderr, code := runCLI(t,
		"decrypt",
		"--private", recipient.privatePath,
		"--passphrase-file", recipient.passphrasePath,
		"--verify-key", signer.publicPath,
		"--replay-state", filepath.Join(dir, "replay.json"),
		"--in", containerPath,
		"--out", outPath,
	)
	if code != 0 {
		t.Fatalf("separate-signer decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "signatureVerified"); got != "true" {
		t.Fatalf("signatureVerified = %s, want true", got)
	}
	if got := outputValue(t, stdout, "signerPrimaryFingerprint"); got != signer.fingerprint {
		t.Fatalf("signer primary fingerprint = %s, want %s", got, signer.fingerprint)
	}
}

func TestDecryptRejectsWrongOrMissingSignature(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	otherDir := filepath.Join(dir, "other")
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatal(err)
	}
	other := newTestIdentity(t, otherDir)
	_, containerPath := encryptSyntheticBundle(t, identity, dir)

	wrongOut := filepath.Join(dir, "wrong-key.json")
	_, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", other.publicPath,
		"--replay-state", filepath.Join(dir, "state-wrong.json"),
		"--in", containerPath,
		"--out", wrongOut,
	)
	if code == 0 {
		t.Fatal("decrypt accepted a signature from a different key")
	}
	if !strings.Contains(stderr, "not the supplied verification key") {
		t.Fatalf("unexpected wrong-signer error: %s", stderr)
	}
	requireMissing(t, wrongOut)

	unsignedPath := filepath.Join(dir, "unsigned.hrtb")
	writeEncryptedContainer(t, identity, readSyntheticBundle(t), unsignedPath)
	unsignedOut := filepath.Join(dir, "unsigned.json")
	_, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", filepath.Join(dir, "state-unsigned.json"),
		"--in", unsignedPath,
		"--out", unsignedOut,
	)
	if code == 0 {
		t.Fatal("decrypt accepted an unsigned backup with --verify-key")
	}
	if !strings.Contains(stderr, "not signed") {
		t.Fatalf("unexpected unsigned error: %s", stderr)
	}
	requireMissing(t, unsignedOut)
}

func TestVerifiedDecryptRejectsOlderReplayUnlessAllowed(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	base := string(readSyntheticBundle(t))
	newerBundle := strings.Replace(base, "2030-01-01T00:00:00.000Z", "2030-02-01T00:00:00.000Z", 1)
	differentBundle := strings.Replace(base, `"id": "tf-event-1"`, `"id": "tf-event-2"`, 1)
	if newerBundle == base || differentBundle == base {
		t.Fatal("synthetic bundle fixture did not contain the expected createdAt/id")
	}
	_, newerContainer := encryptBundleText(t, identity, dir, "newer", newerBundle)
	_, baseContainer := encryptBundleText(t, identity, dir, "base", base)
	_, differentContainer := encryptBundleText(t, identity, dir, "different", differentBundle)

	statePath := filepath.Join(dir, "state.json")
	outNewer := filepath.Join(dir, "decrypted-newer.json")
	stdout, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", statePath,
		"--in", newerContainer,
		"--out", outNewer,
	)
	if code != 0 {
		t.Fatalf("newer decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "replayStatus"); got != "new" {
		t.Fatalf("newer replay status = %s, want new", got)
	}

	outOld := filepath.Join(dir, "old.json")
	_, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", statePath,
		"--in", baseContainer,
		"--out", outOld,
	)
	if code == 0 {
		t.Fatal("decrypt accepted an older verified backup")
	}
	if !strings.Contains(stderr, "older than the newest verified backup") {
		t.Fatalf("unexpected older-backup error: %s", stderr)
	}
	requireMissing(t, outOld)

	stdout, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", statePath,
		"--allow-older",
		"--in", baseContainer,
		"--out", outOld,
	)
	if code != 0 {
		t.Fatalf("--allow-older decrypt failed with code %d: %s", code, stderr)
	}
	if got := outputValue(t, stdout, "replayStatus"); got != "older-allowed" {
		t.Fatalf("allowed replay status = %s, want older-allowed", got)
	}

	// The override must not lower the high-water mark.
	state, err := loadReplayState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Signers[identity.fingerprint].LastCreatedAt; got != "2030-02-01T00:00:00.000Z" {
		t.Fatalf("high-water mark = %q, want the newer backup", got)
	}

	// Same createdAt with different content is a conflict, not a repeat.
	conflictState := filepath.Join(dir, "conflict-state.json")
	outBase := filepath.Join(dir, "decrypted-base.json")
	if _, stderr, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", conflictState,
		"--in", baseContainer,
		"--out", outBase,
	); code != 0 {
		t.Fatalf("baseline decrypt failed: %s", stderr)
	}
	conflictOut := filepath.Join(dir, "conflict.json")
	_, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--verify-key", identity.publicPath,
		"--replay-state", conflictState,
		"--in", differentContainer,
		"--out", conflictOut,
	)
	if code == 0 {
		t.Fatal("decrypt accepted same-time different-content bundle")
	}
	if !strings.Contains(stderr, "different content") {
		t.Fatalf("unexpected conflict error: %s", stderr)
	}
	requireMissing(t, conflictOut)
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

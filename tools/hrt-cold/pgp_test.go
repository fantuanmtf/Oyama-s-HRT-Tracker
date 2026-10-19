package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	gopenpgp "github.com/ProtonMail/gopenpgp/v3/crypto"
)

func testPublicEntity(t *testing.T, identity *testIdentity) *openpgp.Entity {
	t.Helper()
	publicKey, err := gopenpgp.NewKeyFromArmored(string(readTestFile(t, identity.publicPath)))
	if err != nil {
		t.Fatal(err)
	}
	entity := publicKey.GetEntity()
	if entity == nil {
		t.Fatal("public key has no OpenPGP entity")
	}
	return entity
}

func encryptPayloadTo(t *testing.T, to, hidden []*openpgp.Entity, plaintext []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := openpgp.Encrypt(
		&buffer,
		to,
		hidden,
		nil,
		nil,
		&packet.Config{DefaultCompressionAlgo: packet.CompressionNone},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// emptyIntegrityProtectedPayload builds PKESK + SEIPD with no literal packet
// inside the encrypted body, followed by arbitrary outer packets. This is the
// shape that previously made go-crypto fall back to the unencrypted outer
// stream after the empty encrypted packet.
func emptyIntegrityProtectedPayload(t *testing.T, identity *testIdentity, trailing []byte) []byte {
	t.Helper()
	entity := testPublicEntity(t, identity)
	encryptionKey, ok := entity.EncryptionKey(time.Now(), nil)
	if !ok {
		t.Fatal("public key has no encryption key")
	}
	sessionKey := make([]byte, packet.CipherAES128.KeySize())
	if _, err := rand.Read(sessionKey); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := packet.SerializeEncryptedKey(
		&buffer,
		encryptionKey.PublicKey,
		packet.CipherAES128,
		sessionKey,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	contents, err := packet.SerializeSymmetricallyEncrypted(
		&buffer,
		packet.CipherAES128,
		false,
		packet.CipherSuite{},
		sessionKey,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Closing without writing produces an integrity-protected encrypted packet
	// with an empty plaintext body and a valid MDC.
	if err := contents.Close(); err != nil {
		t.Fatal(err)
	}
	buffer.Write(trailing)
	return buffer.Bytes()
}

func markerPayload(t *testing.T, payload []byte, afterFirstPacket bool) []byte {
	t.Helper()
	var marker bytes.Buffer
	if err := packet.SerializeMarker(&marker); err != nil {
		t.Fatal(err)
	}
	if !afterFirstPacket {
		return append(marker.Bytes(), payload...)
	}
	reader := bytes.NewReader(payload)
	if _, err := packet.Read(reader); err != nil {
		t.Fatal(err)
	}
	consumed := len(payload) - reader.Len()
	var buffer bytes.Buffer
	buffer.Write(payload[:consumed])
	buffer.Write(marker.Bytes())
	buffer.Write(payload[consumed:])
	return buffer.Bytes()
}

func splitFirstPacket(t *testing.T, payload []byte) (first, remainder []byte) {
	t.Helper()
	reader := bytes.NewReader(payload)
	if _, err := packet.Read(reader); err != nil {
		t.Fatal(err)
	}
	consumed := len(payload) - reader.Len()
	return payload[:consumed], payload[consumed:]
}

func TestValidateOuterPacketSequence(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	valid := encryptPayloadTo(
		t,
		[]*openpgp.Entity{testPublicEntity(t, identity)},
		nil,
		readSyntheticBundle(t),
	)
	pkesk, encrypted := splitFirstPacket(t, valid)
	var marker bytes.Buffer
	if err := packet.SerializeMarker(&marker); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		payload []byte
		wantErr error
	}{
		{name: "valid", payload: valid},
		{name: "marker-prefix", payload: markerPayload(t, valid, false)},
		{name: "marker-between-pkesk-and-encrypted", payload: markerPayload(t, valid, true)},
		{name: "no-recipient", payload: encrypted, wantErr: errNoRecipient},
		{name: "no-encrypted-packet", payload: pkesk, wantErr: errNoEncryptedPayload},
		{name: "empty-payload", payload: nil, wantErr: errNoEncryptedPayload},
		{name: "trailing-literal", payload: append(append([]byte(nil), valid...), literalPayload(t)...), wantErr: errTrailingPacket},
		{name: "second-encrypted-packet", payload: append(append([]byte(nil), valid...), encrypted...), wantErr: errMultipleEncrypted},
		{name: "marker-after-encrypted", payload: append(append([]byte(nil), valid...), marker.Bytes()...), wantErr: errTrailingPacket},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipients, err := validateOuterPacketSequence(test.payload)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(recipients) != 1 {
				t.Fatalf("recipient count = %d, want 1", len(recipients))
			}
		})
	}
}

func TestDecryptAndInspectRejectEmptyEncryptedPacketWithTrailingPackets(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	tests := []struct {
		name     string
		trailing func(*testing.T) []byte
	}{
		{name: "literal", trailing: literalPayload},
		{name: "compressed", trailing: compressedPlaintextPayload},
		{name: "signed", trailing: func(t *testing.T) []byte { return signedOnlyPayload(t, identity) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := emptyIntegrityProtectedPayload(t, identity, test.trailing(t))
			if _, err := validateOuterPacketSequence(payload); !errors.Is(err, errTrailingPacket) {
				t.Fatalf("validator error = %v, want %v", err, errTrailingPacket)
			}

			containerPath := filepath.Join(dir, "trailing-"+test.name+".hrtb")
			writeContainerPayload(t, payload, containerPath)

			outputPath := filepath.Join(dir, "trailing-"+test.name+".json")
			_, stderr, code := runCLI(t,
				"decrypt",
				"--private", identity.privatePath,
				"--passphrase-file", identity.passphrasePath,
				"--in", containerPath,
				"--out", outputPath,
			)
			if code == 0 {
				t.Fatal("decrypt accepted an empty encrypted packet with a trailing plaintext packet")
			}
			if !strings.Contains(stderr, "after the encrypted data packet") {
				t.Fatalf("unexpected decrypt error: %s", stderr)
			}
			requireMissing(t, outputPath)

			_, stderr, code = runCLI(t, "inspect", "--in", containerPath)
			if code == 0 {
				t.Fatal("inspect accepted an empty encrypted packet with a trailing plaintext packet")
			}
			if !strings.Contains(stderr, "after the encrypted data packet") {
				t.Fatalf("unexpected inspect error: %s", stderr)
			}
			assertNoTempFiles(t, dir)
		})
	}
}

func TestDecryptRejectsEmptyEncryptedPacketWithoutTrailingData(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	payload := emptyIntegrityProtectedPayload(t, identity, nil)
	if _, err := validateOuterPacketSequence(payload); err != nil {
		t.Fatalf("outer sequence should look structurally valid: %v", err)
	}
	containerPath := filepath.Join(dir, "empty-encrypted.hrtb")
	writeContainerPayload(t, payload, containerPath)
	outputPath := filepath.Join(dir, "empty-encrypted.json")
	_, _, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt accepted an encrypted payload with no literal data")
	}
	requireMissing(t, outputPath)
	assertNoTempFiles(t, dir)
}

func TestMultiRecipientEncryptedMessageIsAccepted(t *testing.T) {
	dirA := t.TempDir()
	identityA := newTestIdentity(t, dirA)
	dirB := t.TempDir()
	identityB := newTestIdentity(t, dirB)
	plaintext := readSyntheticBundle(t)
	payload := encryptPayloadTo(
		t,
		[]*openpgp.Entity{testPublicEntity(t, identityA), testPublicEntity(t, identityB)},
		nil,
		plaintext,
	)
	recipients, err := validateOuterPacketSequence(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 2 {
		t.Fatalf("recipient count = %d, want 2", len(recipients))
	}

	containerPath := filepath.Join(dirA, "multi-recipient.hrtb")
	writeContainerPayload(t, payload, containerPath)
	stdout, stderr, code := runCLI(t, "inspect", "--in", containerPath)
	if code != 0 {
		t.Fatalf("inspect failed with code %d: %s", code, stderr)
	}
	if outputValue(t, stdout, "recipientCount") != "2" {
		t.Fatalf("unexpected recipient count: %s", stdout)
	}
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect must not claim recipient verification: %s", stdout)
	}

	for _, identity := range []*testIdentity{identityA, identityB} {
		outputPath := filepath.Join(t.TempDir(), "decrypted.json")
		_, stderr, code := runCLI(t,
			"decrypt",
			"--private", identity.privatePath,
			"--passphrase-file", identity.passphrasePath,
			"--in", containerPath,
			"--out", outputPath,
		)
		if code != 0 {
			t.Fatalf("decrypt with one of the recipients failed with code %d: %s", code, stderr)
		}
		if !bytes.Equal(readTestFile(t, outputPath), plaintext) {
			t.Fatal("multi-recipient decryption changed the plaintext")
		}
	}
}

func TestHiddenRecipientAndMarkerPackets(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	plaintext := readSyntheticBundle(t)
	payload := encryptPayloadTo(t, nil, []*openpgp.Entity{testPublicEntity(t, identity)}, plaintext)
	payload = markerPayload(t, payload, false)
	payload = markerPayload(t, payload, true)

	recipients, err := validateOuterPacketSequence(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 1 {
		t.Fatalf("recipient count = %d, want 1", len(recipients))
	}
	if recipients[0].AdvertisedKeyID != 0 || recipients[0].AdvertisedFingerprint != "" {
		t.Fatalf("hidden recipient leaked metadata: %+v", recipients[0])
	}

	containerPath := filepath.Join(dir, "hidden-recipient.hrtb")
	writeContainerPayload(t, payload, containerPath)
	stdout, stderr, code := runCLI(t, "inspect", "--in", containerPath)
	if code != 0 {
		t.Fatalf("inspect failed with code %d: %s", code, stderr)
	}
	if outputValue(t, stdout, "recipientCount") != "1" {
		t.Fatalf("unexpected recipient count: %s", stdout)
	}
	if outputValue(t, stdout, "advertisedRecipientKeyID") != "0000000000000000" {
		t.Fatalf("unexpected hidden-recipient key ID: %s", stdout)
	}
	if outputValue(t, stdout, "recipientVerified") != "false" {
		t.Fatalf("inspect must not claim recipient verification: %s", stdout)
	}

	outputPath := filepath.Join(dir, "decrypted.json")
	_, stderr, code = runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code != 0 {
		t.Fatalf("decrypt with a hidden recipient failed with code %d: %s", code, stderr)
	}
	if !bytes.Equal(readTestFile(t, outputPath), plaintext) {
		t.Fatal("hidden-recipient decryption changed the plaintext")
	}
	assertNoTempFiles(t, dir)
}

func TestTrailingPacketDoesNotCreateOutputFile(t *testing.T) {
	dir := t.TempDir()
	identity := newTestIdentity(t, dir)
	payload := emptyIntegrityProtectedPayload(t, identity, literalPayload(t))
	containerPath := filepath.Join(dir, "forged-trailing.hrtb")
	writeContainerPayload(t, payload, containerPath)
	outputPath := filepath.Join(dir, "forged-trailing.json")

	_, _, code := runCLI(t,
		"decrypt",
		"--private", identity.privatePath,
		"--passphrase-file", identity.passphrasePath,
		"--in", containerPath,
		"--out", outputPath,
	)
	if code == 0 {
		t.Fatal("decrypt accepted a forged trailing plaintext packet")
	}
	requireMissing(t, outputPath)
	if _, err := os.Stat(outputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output file exists after rejected decryption: %v", err)
	}
	assertNoTempFiles(t, dir)
}

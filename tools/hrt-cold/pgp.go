package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/ProtonMail/gopenpgp/v3/constants"
	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

var (
	errPayloadNotEncrypted = errors.New("container payload is not an encrypted OpenPGP message")
	errNoEncryptedPayload  = errors.New("container payload has no encrypted data packet")
	errNoRecipient         = errors.New("container payload has no public-key recipient")
)

type recipientInfo struct {
	AdvertisedKeyID       uint64
	AdvertisedFingerprint string
}

func generateLockedKey(passphrase []byte) (privateArmor, publicArmor, fingerprint string, err error) {
	pgp := crypto.PGP()
	key, err := pgp.KeyGeneration().
		AddUserId("Oyama HRT Cold Backup", "hrt-cold@example.invalid").
		New().
		GenerateKey()
	if err != nil {
		return "", "", "", fmt.Errorf("generate key: %w", err)
	}
	locked, err := pgp.LockKey(key, passphrase)
	if err != nil {
		return "", "", "", fmt.Errorf("lock private key: %w", err)
	}
	defer locked.ClearPrivateParams()

	fingerprint = locked.GetFingerprint()
	privateArmor, err = locked.Armor()
	if err != nil {
		return "", "", "", fmt.Errorf("armor private key: %w", err)
	}
	publicArmor, err = locked.GetArmoredPublicKey()
	if err != nil {
		return "", "", "", fmt.Errorf("armor public key: %w", err)
	}
	return privateArmor, publicArmor, fingerprint, nil
}

func encryptBundle(recipientArmor string, plaintext []byte) ([]byte, string, error) {
	if len(plaintext) == 0 {
		return nil, "", errors.New("refusing to encrypt an empty bundle")
	}
	publicKey, err := crypto.NewKeyFromArmored(strings.TrimSpace(recipientArmor))
	if err != nil {
		return nil, "", fmt.Errorf("parse recipient public key: %w", err)
	}
	now := time.Now().Unix()
	if !publicKey.CanEncrypt(now) {
		return nil, "", errors.New("recipient key cannot encrypt")
	}
	if publicKey.IsExpired(now) {
		return nil, "", errors.New("recipient key is expired")
	}
	if publicKey.IsRevoked(now) {
		return nil, "", errors.New("recipient key is revoked")
	}

	pgp := crypto.PGP()
	encryption, err := pgp.Encryption().
		Recipient(publicKey).
		CompressWith(constants.NoCompression).
		New()
	if err != nil {
		return nil, "", fmt.Errorf("create encryption handle: %w", err)
	}
	message, err := encryption.Encrypt(plaintext)
	if err != nil {
		return nil, "", fmt.Errorf("encrypt bundle: %w", err)
	}
	containerBytes, err := makeContainer(message.Bytes())
	if err != nil {
		return nil, "", err
	}
	return containerBytes, publicKey.GetFingerprint(), nil
}

func decryptContainer(privateArmor string, passphrase, payload []byte) ([]byte, string, error) {
	privateKey, err := crypto.NewPrivateKeyFromArmored(strings.TrimSpace(privateArmor), passphrase)
	if err != nil {
		return nil, "", fmt.Errorf("unlock private key (wrong passphrase or unsupported key): %w", err)
	}
	defer privateKey.ClearPrivateParams()

	now := time.Now().Unix()
	if privateKey.IsExpired(now) {
		return nil, "", errors.New("private key is expired")
	}
	if privateKey.IsRevoked(now) {
		return nil, "", errors.New("private key is revoked")
	}
	entity := privateKey.GetEntity()
	if entity == nil {
		return nil, "", errors.New("private key has no OpenPGP entity")
	}

	// Decrypt through go-crypto directly so the message metadata can be
	// checked. gopenpgp does not expose whether the parsed message was
	// actually encrypted or which key decrypted it, and a literal,
	// compressed-plaintext, or signed-only message would otherwise be
	// accepted without the private key taking part.
	decompressedLimit := int64(maxBundleBytes)
	config := &packet.Config{
		MaxDecompressedMessageSize: &decompressedLimit,
		CheckPacketSequence:        packet.BoolPointer(true),
		CheckIntendedRecipients:    packet.BoolPointer(true),
	}
	message, err := openpgp.ReadMessage(
		bytes.NewReader(payload),
		openpgp.EntityList{entity},
		nil,
		config,
	)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt (wrong key or corrupt container): %w", err)
	}
	if !message.IsEncrypted {
		return nil, "", fmt.Errorf(
			"%w: literal, compressed-plaintext, and signed-only payloads are rejected",
			errPayloadNotEncrypted,
		)
	}
	if message.DecryptedWith.Entity == nil || message.DecryptedWith.PrivateKey == nil {
		return nil, "", errors.New("container payload was not decrypted with the supplied private key")
	}
	if message.LiteralData == nil || message.UnverifiedBody == nil {
		return nil, "", errors.New("encrypted OpenPGP payload has no literal data")
	}

	// Reading to EOF is required: the MDC/integrity check runs when the
	// encrypted packet is closed at the end of the plaintext body.
	plaintext, err := io.ReadAll(message.UnverifiedBody)
	if err != nil {
		return nil, "", fmt.Errorf("verify encrypted payload (tampered, truncated, or corrupt container): %w", err)
	}
	if len(plaintext) == 0 {
		return nil, "", fmt.Errorf("%w: decrypted payload is empty", errBundleSchema)
	}
	if len(plaintext) > maxBundleBytes {
		return nil, "", fmt.Errorf("%w: decrypted payload is %d bytes", errTooLarge, len(plaintext))
	}
	return plaintext, privateKey.GetFingerprint(), nil
}

// inspectRecipients reads the unauthenticated public-key-encrypted session-key
// packets (PKESK) at the start of the payload. It never decrypts the session
// key or the message, and a PKESK KeyID must not be treated as proof that the
// container is encrypted to a particular key.
func inspectRecipients(payload []byte) ([]recipientInfo, error) {
	reader := bytes.NewReader(payload)
	var recipients []recipientInfo
	for {
		parsed, err := packet.Read(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse OpenPGP packet: %w", err)
		}
		switch p := parsed.(type) {
		case *packet.EncryptedKey:
			info := recipientInfo{AdvertisedKeyID: p.KeyId}
			if len(p.KeyFingerprint) > 0 {
				info.AdvertisedFingerprint = hex.EncodeToString(p.KeyFingerprint)
			}
			recipients = append(recipients, info)
		case *packet.SymmetricallyEncrypted:
			if !p.IntegrityProtected {
				return nil, errors.New("container payload is not integrity protected")
			}
			if len(recipients) == 0 {
				return nil, errNoRecipient
			}
			return recipients, nil
		case *packet.AEADEncrypted:
			if len(recipients) == 0 {
				return nil, errNoRecipient
			}
			return recipients, nil
		case *packet.SymmetricKeyEncrypted:
			return nil, errors.New("container payload is passphrase-encrypted, not public-key encrypted")
		default:
			return nil, fmt.Errorf(
				"%w: unexpected %T packet",
				errPayloadNotEncrypted,
				parsed,
			)
		}
	}
	return nil, errNoEncryptedPayload
}

// matchAdvertisedRecipient maps unauthenticated PKESK key IDs or fingerprints
// to the supplied public key. A match is informational only; it does not prove
// that the container is encrypted to that key or that the key can decrypt it.
func matchAdvertisedRecipient(recipientArmor string, recipients []recipientInfo) ([]string, string, error) {
	publicKey, err := crypto.NewKeyFromArmored(strings.TrimSpace(recipientArmor))
	if err != nil {
		return nil, "", fmt.Errorf("parse recipient public key: %w", err)
	}
	entity := publicKey.GetEntity()
	if entity == nil || entity.PrimaryKey == nil {
		return nil, "", errors.New("recipient public key has no primary key")
	}
	primaryFingerprint := publicKey.GetFingerprint()
	type candidate struct {
		keyID       uint64
		fingerprint string
	}
	candidates := []candidate{{
		keyID:       entity.PrimaryKey.KeyId,
		fingerprint: primaryFingerprint,
	}}
	for _, subkey := range entity.Subkeys {
		if subkey.PublicKey == nil {
			continue
		}
		candidates = append(candidates, candidate{
			keyID:       subkey.PublicKey.KeyId,
			fingerprint: hex.EncodeToString(subkey.PublicKey.Fingerprint),
		})
	}

	seen := make(map[string]bool)
	var matches []string
	for _, recipient := range recipients {
		for _, candidate := range candidates {
			matched := false
			if recipient.AdvertisedFingerprint != "" {
				matched = strings.EqualFold(recipient.AdvertisedFingerprint, candidate.fingerprint)
			} else {
				matched = recipient.AdvertisedKeyID == candidate.keyID
			}
			if matched && !seen[candidate.fingerprint] {
				seen[candidate.fingerprint] = true
				matches = append(matches, candidate.fingerprint)
			}
		}
	}
	return matches, primaryFingerprint, nil
}

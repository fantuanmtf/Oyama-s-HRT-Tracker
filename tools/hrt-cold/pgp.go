package main

import (
	"bytes"
	"crypto/sha256"
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
	errTrailingPacket      = errors.New("container payload has a packet after the encrypted data packet")
	errMultipleEncrypted   = errors.New("container payload has more than one encrypted data packet")
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

// encryptBundle encrypts the bundle and, when signerArmor is non-empty, signs
// the plaintext inside the encryption. The signature therefore covers every
// record, the bundle createdAt, and any future fields without changing the
// website envelope or the .hrtb container.
func encryptBundle(
	recipientArmor, signerArmor string,
	signerPassphrase, plaintext []byte,
) (containerBytes []byte, recipientFingerprint, signerFingerprint, contentSHA256 string, err error) {
	if len(plaintext) == 0 {
		return nil, "", "", "", errors.New("refusing to encrypt an empty bundle")
	}
	publicKey, err := crypto.NewKeyFromArmored(strings.TrimSpace(recipientArmor))
	if err != nil {
		return nil, "", "", "", fmt.Errorf("parse recipient public key: %w", err)
	}
	now := time.Now().Unix()
	if !publicKey.CanEncrypt(now) {
		return nil, "", "", "", errors.New("recipient key cannot encrypt")
	}
	if publicKey.IsExpired(now) {
		return nil, "", "", "", errors.New("recipient key is expired")
	}
	if publicKey.IsRevoked(now) {
		return nil, "", "", "", errors.New("recipient key is revoked")
	}

	var signerKey *crypto.Key
	if signerArmor != "" {
		signerKey, err = crypto.NewPrivateKeyFromArmored(strings.TrimSpace(signerArmor), signerPassphrase)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("unlock signer key (wrong passphrase or unsupported key): %w", err)
		}
		defer signerKey.ClearPrivateParams()
		if signerKey.IsExpired(now) {
			return nil, "", "", "", errors.New("signer key is expired")
		}
		if signerKey.IsRevoked(now) {
			return nil, "", "", "", errors.New("signer key is revoked")
		}
		signerEntity := signerKey.GetEntity()
		if signerEntity == nil {
			return nil, "", "", "", errors.New("signer key has no OpenPGP entity")
		}
		if _, ok := signerEntity.SigningKey(time.Now(), nil); !ok {
			return nil, "", "", "", errors.New("signer key cannot sign")
		}
		signerFingerprint = signerKey.GetFingerprint()
	}

	hash := sha256.Sum256(plaintext)
	contentSHA256 = hex.EncodeToString(hash[:])

	pgp := crypto.PGP()
	builder := pgp.Encryption().
		Recipient(publicKey).
		CompressWith(constants.NoCompression)
	if signerKey != nil {
		builder = builder.SigningKey(signerKey)
	}
	encryption, err := builder.New()
	if err != nil {
		return nil, "", "", "", fmt.Errorf("create encryption handle: %w", err)
	}
	message, err := encryption.Encrypt(plaintext)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("encrypt bundle: %w", err)
	}
	containerBytes, err = makeContainer(message.Bytes())
	if err != nil {
		return nil, "", "", "", err
	}
	return containerBytes, publicKey.GetFingerprint(), signerFingerprint, contentSHA256, nil
}

type decryptResult struct {
	Plaintext                []byte
	PrivateKeyFingerprint    string
	SignaturePresent         bool
	SignatureVerified        bool
	SignerFingerprint        string
	SignerPrimaryFingerprint string
	ContentSHA256            string
}

// decryptContainer decrypts and validates the OpenPGP payload. When
// verifyArmor is non-empty the message must carry a valid signature from that
// key; a signature from any other key fails closed. The signature is checked
// over the plaintext, so it covers the complete-backup envelope and every
// record inside it.
func decryptContainer(
	privateArmor string,
	passphrase, payload []byte,
	verifyArmor string,
) (*decryptResult, error) {
	// Validate the unencrypted outer packet sequence before unlocking the key
	// or parsing the message. go-crypto's packet reader can fall back to the
	// outer stream when the encrypted payload is empty, so trailing plaintext
	// packets must be rejected here rather than trusted after ReadMessage.
	if _, err := validateOuterPacketSequence(payload); err != nil {
		return nil, err
	}

	privateKey, err := crypto.NewPrivateKeyFromArmored(strings.TrimSpace(privateArmor), passphrase)
	if err != nil {
		return nil, fmt.Errorf("unlock private key (wrong passphrase or unsupported key): %w", err)
	}
	defer privateKey.ClearPrivateParams()

	now := time.Now().Unix()
	if privateKey.IsExpired(now) {
		return nil, errors.New("private key is expired")
	}
	if privateKey.IsRevoked(now) {
		return nil, errors.New("private key is revoked")
	}
	entity := privateKey.GetEntity()
	if entity == nil {
		return nil, errors.New("private key has no OpenPGP entity")
	}
	keyring := openpgp.EntityList{entity}

	var verifyEntity *openpgp.Entity
	if verifyArmor != "" {
		verifyKey, err := crypto.NewKeyFromArmored(strings.TrimSpace(verifyArmor))
		if err != nil {
			return nil, fmt.Errorf("parse verification public key: %w", err)
		}
		if verifyKey.IsExpired(now) {
			return nil, errors.New("verification key is expired")
		}
		if verifyKey.IsRevoked(now) {
			return nil, errors.New("verification key is revoked")
		}
		if !verifyKey.CanVerify(now) {
			return nil, errors.New("verification key cannot verify signatures")
		}
		verifyEntity = verifyKey.GetEntity()
		if verifyEntity == nil {
			return nil, errors.New("verification key has no OpenPGP entity")
		}
		keyring = append(keyring, verifyEntity)
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
		keyring,
		nil,
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("decrypt (wrong key or corrupt container): %w", err)
	}
	if !message.IsEncrypted {
		return nil, fmt.Errorf(
			"%w: literal, compressed-plaintext, and signed-only payloads are rejected",
			errPayloadNotEncrypted,
		)
	}
	if message.DecryptedWith.Entity == nil || message.DecryptedWith.PrivateKey == nil {
		return nil, errors.New("container payload was not decrypted with the supplied private key")
	}
	if message.LiteralData == nil || message.UnverifiedBody == nil {
		return nil, errors.New("encrypted OpenPGP payload has no literal data")
	}

	// Reading to EOF is required: both the MDC/integrity check and the inner
	// signature verification finish when the encrypted packet closes.
	plaintext, err := io.ReadAll(message.UnverifiedBody)
	if err != nil {
		return nil, fmt.Errorf("verify encrypted payload (tampered, truncated, or corrupt container): %w", err)
	}
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("%w: decrypted payload is empty", errBundleSchema)
	}
	if len(plaintext) > maxBundleBytes {
		return nil, fmt.Errorf("%w: decrypted payload is %d bytes", errTooLarge, len(plaintext))
	}

	result := &decryptResult{
		Plaintext:             plaintext,
		PrivateKeyFingerprint: privateKey.GetFingerprint(),
		SignaturePresent:      message.IsSigned,
	}
	hash := sha256.Sum256(plaintext)
	result.ContentSHA256 = hex.EncodeToString(hash[:])

	if verifyArmor != "" {
		if !message.IsSigned {
			return nil, errors.New("backup is not signed, but --verify-key was supplied")
		}
		if message.SignatureError != nil {
			return nil, fmt.Errorf("signature verification failed: %w", message.SignatureError)
		}
		if message.SignedBy == nil || message.SignedBy.PublicKey == nil {
			return nil, errors.New("backup has no verified signer key")
		}
		if !keyMatchesEntity(verifyEntity, message.SignedBy.PublicKey.Fingerprint) {
			return nil, fmt.Errorf(
				"backup is signed by key %s, not the supplied verification key",
				hex.EncodeToString(message.SignedBy.PublicKey.Fingerprint),
			)
		}
		result.SignatureVerified = true
		result.SignerFingerprint = hex.EncodeToString(message.SignedBy.PublicKey.Fingerprint)
		if message.SignedBy.Entity != nil && message.SignedBy.Entity.PrimaryKey != nil {
			result.SignerPrimaryFingerprint = hex.EncodeToString(message.SignedBy.Entity.PrimaryKey.Fingerprint)
		}
	}
	return result, nil
}

// keyMatchesEntity reports whether fingerprint belongs to the entity's primary
// key or one of its subkeys.
func keyMatchesEntity(entity *openpgp.Entity, fingerprint []byte) bool {
	if entity == nil || entity.PrimaryKey == nil || len(fingerprint) == 0 {
		return false
	}
	if bytes.Equal(entity.PrimaryKey.Fingerprint, fingerprint) {
		return true
	}
	for _, subkey := range entity.Subkeys {
		if subkey.PublicKey != nil && bytes.Equal(subkey.PublicKey.Fingerprint, fingerprint) {
			return true
		}
	}
	return false
}

// validateOuterPacketSequence parses the unencrypted outer packet layer of an
// OpenPGP message without decrypting it. The accepted sequence is:
//
//	Marker* PKESK+ (SymmetricallyEncrypted[IntegrityProtected] | AEADEncrypted) EOF
//
// Marker packets are ignored. At least one public-key encrypted session-key
// packet (PKESK) and exactly one encrypted data packet are required, and the
// payload must end immediately after the encrypted data packet. Rejecting
// trailing packets prevents go-crypto from treating an unencrypted packet after
// an empty encrypted packet as the message body.
//
// The returned recipient identifiers are unauthenticated PKESK advertisements;
// they must not be treated as proof that the container is encrypted to a
// particular key.
func validateOuterPacketSequence(payload []byte) ([]recipientInfo, error) {
	reader := bytes.NewReader(payload)
	var recipients []recipientInfo
	encryptedSeen := false

	for {
		parsed, err := packet.Read(reader)
		if errors.Is(err, io.EOF) {
			if !encryptedSeen {
				return nil, errNoEncryptedPayload
			}
			return recipients, nil
		}
		if err != nil {
			if encryptedSeen {
				return nil, fmt.Errorf("%w: %v", errTrailingPacket, err)
			}
			return nil, fmt.Errorf("parse OpenPGP packet: %w", err)
		}
		if encryptedSeen {
			switch parsed.(type) {
			case *packet.SymmetricallyEncrypted, *packet.AEADEncrypted:
				return nil, errMultipleEncrypted
			default:
				return nil, fmt.Errorf("%w: unexpected %T packet", errTrailingPacket, parsed)
			}
		}

		switch p := parsed.(type) {
		case *packet.Marker:
			// packet.Read has already validated the "PGP" marker contents.
			// Marker packets are legal in the outer sequence and carry no
			// message data.
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
			// Consume the encrypted packet body so the next read observes
			// either EOF or an actual packet outside the encrypted data.
			if _, err := io.Copy(io.Discard, p.Contents); err != nil {
				return nil, fmt.Errorf("read encrypted data packet: %w", err)
			}
			encryptedSeen = true
		case *packet.AEADEncrypted:
			if len(recipients) == 0 {
				return nil, errNoRecipient
			}
			if _, err := io.Copy(io.Discard, p.Contents); err != nil {
				return nil, fmt.Errorf("read encrypted data packet: %w", err)
			}
			encryptedSeen = true
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

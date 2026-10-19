package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	// maxContainerBytes is the total .hrtb size limit, including the 16-byte
	// header. It is intentionally small enough to avoid unbounded parsing.
	maxContainerBytes = 16 << 20
	// maxBundleBytes leaves room for the OpenPGP encryption overhead inside a
	// container whose total size is capped at maxContainerBytes.
	maxBundleBytes     = 15 << 20
	maxKeyBytes        = 1 << 20
	maxPassphraseBytes = 4096
)

var (
	errOutputExists   = errors.New("output already exists")
	errTooLarge       = errors.New("input exceeds size limit")
	errUnsafePassfile = errors.New("passphrase file is not private enough")
	errHelp           = errors.New("help requested")
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "keygen":
		err = cmdKeygen(args[1:], stdout, stderr)
	case "encrypt":
		err = cmdEncrypt(args[1:], stdout, stderr)
	case "decrypt":
		err = cmdDecrypt(args[1:], stdout, stderr)
	case "inspect":
		err = cmdInspect(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		usage(stderr)
		return 2
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, errHelp) {
		return 0
	}
	fmt.Fprintln(stderr, "error:", err)
	return 1
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `usage:
  hrt-cold keygen  --private-out FILE --public-out FILE --passphrase-file FILE
  hrt-cold encrypt --recipient FILE [--signer-private FILE --signer-passphrase-file FILE]
                   --in FILE --out FILE
  hrt-cold decrypt --private FILE --passphrase-file FILE [--verify-key FILE]
                   [--allow-older] [--no-replay-check] [--replay-state FILE]
                   --in FILE --out FILE
  hrt-cold inspect --in FILE [--recipient FILE]

Signing is recommended: encrypt signs the plaintext with the signer key, and
decrypt verifies it when --verify-key is supplied. Verified decrypts also
check a local high-water mark to refuse an older backup (anti-replay).

All commands are offline. The .hrtb container format is experimental and is
not a stable production format.`)
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelp
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

func cmdKeygen(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("keygen", stderr)
	privateOut := fs.String("private-out", "", "armored private key output")
	publicOut := fs.String("public-out", "", "armored public key output")
	passFile := fs.String("passphrase-file", "", "0600 (or stricter) file containing the private-key passphrase")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *privateOut == "" || *publicOut == "" || *passFile == "" {
		return errors.New("keygen requires --private-out, --public-out, and --passphrase-file")
	}
	if samePath(*privateOut, *publicOut) {
		return errors.New("--private-out and --public-out must be different files")
	}
	passphrase, err := readPassphrase(*passFile)
	if err != nil {
		return err
	}
	if note := passphraseFilePlatformNote(); note != "" {
		fmt.Fprintln(stderr, note)
	}
	privateArmor, publicArmor, fingerprint, err := generateLockedKey(passphrase)
	if err != nil {
		return err
	}
	if err := atomicWrite(*privateOut, []byte(privateArmor+"\n"), 0o600); err != nil {
		return err
	}
	if err := atomicWrite(*publicOut, []byte(publicArmor+"\n"), 0o644); err != nil {
		// The pair is incomplete without both files. Remove only the private
		// file this invocation just published.
		if removeErr := os.Remove(*privateOut); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("%v (also failed to remove private key: %w)", err, removeErr)
		}
		return err
	}
	fmt.Fprintf(stdout, "fingerprint: %s\n", fingerprint)
	return nil
}

func cmdEncrypt(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("encrypt", stderr)
	recipientPath := fs.String("recipient", "", "armored public key of the recipient")
	signerPath := fs.String("signer-private", "", "armored private signing key (recommended)")
	signerPassFile := fs.String("signer-passphrase-file", "", "0600 (or stricter) file containing the signing-key passphrase")
	inPath := fs.String("in", "", "website complete-backup JSON bundle")
	outPath := fs.String("out", "", ".hrtb output")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *recipientPath == "" || *inPath == "" || *outPath == "" {
		return errors.New("encrypt requires --recipient, --in, and --out")
	}
	if (*signerPath == "") != (*signerPassFile == "") {
		return errors.New("--signer-private and --signer-passphrase-file must be supplied together")
	}
	if err := ensureOutputMissing(*outPath); err != nil {
		return err
	}
	bundle, err := readLimited(*inPath, maxBundleBytes)
	if err != nil {
		return err
	}
	info, err := validateBundle(bundle)
	if err != nil {
		return fmt.Errorf("invalid complete-backup bundle: %w", err)
	}
	recipientArmor, err := readLimited(*recipientPath, maxKeyBytes)
	if err != nil {
		return err
	}

	var signerArmor string
	var signerPassphrase []byte
	if *signerPath != "" {
		signer, err := readLimited(*signerPath, maxKeyBytes)
		if err != nil {
			return err
		}
		signerArmor = string(signer)
		signerPassphrase, err = readPassphrase(*signerPassFile)
		if err != nil {
			return err
		}
		if note := passphraseFilePlatformNote(); note != "" {
			fmt.Fprintln(stderr, note)
		}
	} else {
		fmt.Fprintln(stderr, "warning: creating an unsigned backup; pass --signer-private and --signer-passphrase-file for provenance")
	}

	container, fingerprint, signerFingerprint, contentSHA256, err := encryptBundle(
		string(recipientArmor), signerArmor, signerPassphrase, bundle,
	)
	if err != nil {
		return err
	}
	if err := atomicWrite(*outPath, container, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "recipientFingerprint: %s\n", fingerprint)
	if signerFingerprint != "" {
		fmt.Fprintf(stdout, "signerFingerprint: %s\n", signerFingerprint)
		fmt.Fprintln(stdout, "signature: signed")
	} else {
		fmt.Fprintln(stdout, "signature: unsigned")
	}
	fmt.Fprintf(stdout, "contentSha256: %s\n", contentSHA256)
	fmt.Fprintf(stdout, "bundleCreatedAt: %s\n", info.CreatedAt)
	fmt.Fprintf(stdout, "bundleMode: %s\n", info.Mode)
	return nil
}

func cmdDecrypt(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("decrypt", stderr)
	privatePath := fs.String("private", "", "armored private key")
	passFile := fs.String("passphrase-file", "", "0600 (or stricter) file containing the private-key passphrase")
	verifyPath := fs.String("verify-key", "", "armored public key that must have signed the backup")
	inPath := fs.String("in", "", ".hrtb input")
	outPath := fs.String("out", "", "validated complete-backup JSON output")
	allowOlder := fs.Bool("allow-older", false, "accept an older or same-time-different verified backup (replay override)")
	noReplay := fs.Bool("no-replay-check", false, "do not read or update the local anti-replay state")
	replayStatePath := fs.String("replay-state", "", "override the local replay state file")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *privatePath == "" || *passFile == "" || *inPath == "" || *outPath == "" {
		return errors.New("decrypt requires --private, --passphrase-file, --in, and --out")
	}
	if err := ensureOutputMissing(*outPath); err != nil {
		return err
	}
	rawContainer, err := readLimited(*inPath, maxContainerBytes)
	if err != nil {
		return err
	}
	container, err := parseContainer(rawContainer)
	if err != nil {
		return err
	}
	passphrase, err := readPassphrase(*passFile)
	if err != nil {
		return err
	}
	if note := passphraseFilePlatformNote(); note != "" {
		fmt.Fprintln(stderr, note)
	}
	privateArmor, err := readLimited(*privatePath, maxKeyBytes)
	if err != nil {
		return err
	}
	var verifyArmor string
	if *verifyPath != "" {
		verify, err := readLimited(*verifyPath, maxKeyBytes)
		if err != nil {
			return err
		}
		verifyArmor = string(verify)
	} else {
		fmt.Fprintln(stderr, "warning: no --verify-key supplied; the backup's origin and signature are not verified")
	}

	result, err := decryptContainer(string(privateArmor), passphrase, container.Payload, verifyArmor)
	if err != nil {
		return err
	}
	info, err := validateBundle(result.Plaintext)
	if err != nil {
		return fmt.Errorf("decrypted payload is not a valid complete-backup bundle: %w", err)
	}

	replayStatus := "disabled"
	var replayPath string
	var state *replayState
	replaySigner := result.SignerPrimaryFingerprint
	if replaySigner == "" {
		replaySigner = result.SignerFingerprint
	}
	if result.SignatureVerified && !*noReplay {
		if *replayStatePath != "" {
			replayPath = *replayStatePath
		} else {
			replayPath, err = defaultReplayStatePath()
			if err != nil {
				return err
			}
		}
		state, err = loadReplayState(replayPath)
		if err != nil {
			return err
		}
		replayStatus, _, err = checkReplay(state, replaySigner, info.CreatedAt, result.ContentSHA256, *allowOlder)
		if err != nil {
			return err
		}
	} else if result.SignatureVerified && *noReplay {
		replayStatus = "check-disabled"
	} else {
		replayStatus = "unsigned"
	}

	if err := atomicWrite(*outPath, result.Plaintext, 0o600); err != nil {
		return err
	}
	if result.SignatureVerified && !*noReplay && (replayStatus == "new" || replayStatus == "newer") {
		state.record(replaySigner, info.CreatedAt, result.ContentSHA256)
		if err := writeReplayState(replayPath, state); err != nil {
			return fmt.Errorf("backup was written to %s, but replay state was not updated: %w", *outPath, err)
		}
	}

	fmt.Fprintf(stdout, "privateKeyFingerprint: %s\n", result.PrivateKeyFingerprint)
	fmt.Fprintf(stdout, "signatureVerified: %t\n", result.SignatureVerified)
	if result.SignatureVerified {
		fmt.Fprintf(stdout, "signerFingerprint: %s\n", result.SignerFingerprint)
		fmt.Fprintf(stdout, "signerPrimaryFingerprint: %s\n", result.SignerPrimaryFingerprint)
	}
	fmt.Fprintf(stdout, "contentSha256: %s\n", result.ContentSHA256)
	fmt.Fprintf(stdout, "bundleCreatedAt: %s\n", info.CreatedAt)
	fmt.Fprintf(stdout, "replayStatus: %s\n", replayStatus)
	return nil
}

func cmdInspect(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("inspect", stderr)
	inPath := fs.String("in", "", ".hrtb input")
	recipientPath := fs.String("recipient", "", "optional armored public key to compare with advertised recipient key IDs (informational only)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *inPath == "" {
		return errors.New("inspect requires --in")
	}
	rawContainer, err := readLimited(*inPath, maxContainerBytes)
	if err != nil {
		return err
	}
	container, err := parseContainer(rawContainer)
	if err != nil {
		return err
	}
	recipients, err := validateOuterPacketSequence(container.Payload)
	if err != nil {
		return err
	}

	var matched []string
	var suppliedFingerprint string
	if *recipientPath != "" {
		recipientArmor, err := readLimited(*recipientPath, maxKeyBytes)
		if err != nil {
			return err
		}
		matched, suppliedFingerprint, err = matchAdvertisedRecipient(string(recipientArmor), recipients)
		if err != nil {
			return err
		}
	}

	sum := sha256.Sum256(container.Payload)
	fmt.Fprintf(stdout, "magic: %s\n", containerMagic)
	fmt.Fprintf(stdout, "containerVersion: %d\n", container.Version)
	fmt.Fprintf(stdout, "containerFlags: %d\n", container.Flags)
	fmt.Fprintf(stdout, "payloadBytes: %d\n", len(container.Payload))
	fmt.Fprintf(stdout, "payloadSHA256: %s\n", hex.EncodeToString(sum[:]))
	fmt.Fprintf(stdout, "recipientCount: %d\n", len(recipients))
	for _, recipient := range recipients {
		fmt.Fprintf(stdout, "advertisedRecipientKeyID: %016x\n", recipient.AdvertisedKeyID)
		if recipient.AdvertisedFingerprint != "" {
			fmt.Fprintf(stdout, "advertisedRecipientFingerprint: %s\n", recipient.AdvertisedFingerprint)
		}
	}
	if suppliedFingerprint != "" {
		fmt.Fprintf(stdout, "suppliedRecipientFingerprint: %s\n", suppliedFingerprint)
		fmt.Fprintf(stdout, "advertisedRecipientMatchesSupplied: %t\n", len(matched) > 0)
		for _, fingerprint := range matched {
			fmt.Fprintf(stdout, "advertisedRecipientMatchFingerprint: %s\n", fingerprint)
		}
	}
	fmt.Fprintln(stdout, "recipientVerified: false")
	fmt.Fprintln(stdout, "decrypted: false")
	return nil
}

func ensureOutputMissing(path string) error {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return fmt.Errorf("%w: %s", errOutputExists, path)
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return err
	}
}

func readPassphrase(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if err := checkPassphraseFileMode(path, info); err != nil {
		return nil, err
	}
	raw, err := readLimited(path, maxPassphraseBytes)
	if err != nil {
		return nil, err
	}
	passphrase := strings.TrimRight(string(raw), "\r\n")
	if passphrase == "" {
		return nil, errors.New("passphrase file is empty")
	}
	if !utf8.ValidString(passphrase) {
		return nil, errors.New("passphrase is not valid UTF-8")
	}
	return []byte(passphrase), nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", errTooLarge, path, limit)
	}
	return raw, nil
}

func samePath(a, b string) bool {
	absoluteA, errA := filepath.Abs(a)
	absoluteB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	cleanA := filepath.Clean(absoluteA)
	cleanB := filepath.Clean(absoluteB)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}

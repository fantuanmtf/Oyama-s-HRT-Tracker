//go:build windows

package main

import (
	"fmt"
	"os"
)

// Go's os.FileMode permission bits are not Windows ACLs: a normal writable
// file reports 0666 and os.Chmod can only toggle the read-only attribute.
// Reject directories/devices, and document that the NTFS ACL is the real
// confidentiality boundary. The tool does not claim to verify that ACL.
func checkPassphraseFileMode(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", errUnsafePassfile, path)
	}
	return nil
}

func passphraseFilePlatformNote() string {
	return "note: Windows NTFS ACLs are the passphrase-file security boundary; Unix 0600 mode bits are not verified on this platform"
}

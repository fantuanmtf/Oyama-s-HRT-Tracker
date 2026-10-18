//go:build !windows

package main

import (
	"fmt"
	"os"
)

func checkPassphraseFileMode(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", errUnsafePassfile, path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: %s has mode %04o; use 0600 or stricter", errUnsafePassfile, path, info.Mode().Perm())
	}
	return nil
}

func passphraseFilePlatformNote() string {
	return ""
}

// +build !linux !amd64

package wal

import (
	"fmt"
	"runtime"
)

// IsBtrfs checks whether the file is in btrfs
func IsBtrfs(path string) bool {
	return false
}

// SetNOCOWFile sets NOCOW flag for file
func SetNOCOWFile(path string) error {
	return fmt.Errorf("wal: unsupported platform %v", runtime.GOOS)
}

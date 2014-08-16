// +build linux,amd64

package wal

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

const (
	// from Linux/include/uapi/linux/magic.h
	BTRFS_SUPER_MAGIC = 0x9123683E

	// from Linux/include/uapi/linux/fs.h
	FS_NOCOW_FL     = 0x00800000
	FS_IOC_GETFLAGS = 0x80086601
	FS_IOC_SETFLAGS = 0x40086602
)

// IsBtrfs checks whether the file is in btrfs
func IsBtrfs(path string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		log.Printf("Failed to statfs: %v", err)
		return false
	}
	if stat.Type != BTRFS_SUPER_MAGIC {
		return false
	}
	return true
}

// SetNOCOWFile sets NOCOW flag for file
func SetNOCOWFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fileinfo, err := file.Stat()
	if err != nil {
		return err
	}
	if fileinfo.IsDir() {
		return fmt.Errorf("wal: btfs skips directory")
	}
	if fileinfo.Size() != 0 {
		return fmt.Errorf("wal: btfs skips nonempty file")
	}

	var attr int
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), FS_IOC_GETFLAGS, uintptr(unsafe.Pointer(&attr))); errno != 0 {
		return errno
	}
	attr |= FS_NOCOW_FL
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), FS_IOC_SETFLAGS, uintptr(unsafe.Pointer(&attr))); errno != 0 {
		return errno
	}
	return nil
}

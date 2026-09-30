//go:build !windows

package logmonitor

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("文件身份信息不可用")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}

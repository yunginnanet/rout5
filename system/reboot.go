//go:build !amd64
// +build !amd64

package system

import "golang.org/x/sys/unix"

func reboot() error {
	return unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

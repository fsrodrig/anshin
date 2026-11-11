package server

import (
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func freebind(network, _ string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		if strings.HasSuffix(network, "6") {
			serr = unix.SetsockoptInt(int(fd), unix.SOL_IPV6, unix.IPV6_FREEBIND, 1)
		} else {
			serr = unix.SetsockoptInt(int(fd), unix.SOL_IP, unix.IP_FREEBIND, 1)
		}
	})
	if err != nil {
		return err
	}
	return serr
}

//go:build !linux

package main

import (
	"errors"
	"net"
)

func listenVsock(uint32) (net.Listener, error) {
	return nil, errors.New("guest vsock runtime is only supported on Linux")
}

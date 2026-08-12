//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func listenVsock(port uint32) (net.Listener, error) {
	fileDescriptor, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("create guest vsock listener: %w", err)
	}
	address := &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: port}
	if err := unix.Bind(fileDescriptor, address); err != nil {
		_ = unix.Close(fileDescriptor)
		return nil, fmt.Errorf("bind guest vsock port %d: %w", port, err)
	}
	if err := unix.Listen(fileDescriptor, 16); err != nil {
		_ = unix.Close(fileDescriptor)
		return nil, fmt.Errorf("listen on guest vsock port %d: %w", port, err)
	}
	// net.FileListener asks the standard library to translate the socket's
	// address family. Go's net package does not recognize AF_VSOCK there, even
	// though the Linux socket itself is valid. Accept it directly instead so the
	// runtime remains a normal net.Listener without a host-side workaround.
	return &vsockListener{fileDescriptor: fileDescriptor, address: vsockAddress{port: port}}, nil
}

type vsockListener struct {
	fileDescriptor int
	address        vsockAddress
	closeOnce      sync.Once
	closeErr       error
}

func (listener *vsockListener) Accept() (net.Conn, error) {
	fileDescriptor, _, err := unix.Accept(listener.fileDescriptor)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), "hdu-station-vsock-connection")
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return nil, fmt.Errorf("wrap guest vsock connection")
	}
	return &vsockConnection{File: file, local: listener.address}, nil
}

func (listener *vsockListener) Close() error {
	listener.closeOnce.Do(func() { listener.closeErr = unix.Close(listener.fileDescriptor) })
	return listener.closeErr
}

func (listener *vsockListener) Addr() net.Addr { return listener.address }

type vsockConnection struct {
	*os.File
	local vsockAddress
}

func (connection *vsockConnection) LocalAddr() net.Addr  { return connection.local }
func (connection *vsockConnection) RemoteAddr() net.Addr { return vsockAddress{} }
func (connection *vsockConnection) SetDeadline(value time.Time) error {
	return connection.File.SetDeadline(value)
}
func (connection *vsockConnection) SetReadDeadline(value time.Time) error {
	return connection.File.SetReadDeadline(value)
}
func (connection *vsockConnection) SetWriteDeadline(value time.Time) error {
	return connection.File.SetWriteDeadline(value)
}

type vsockAddress struct{ port uint32 }

func (address vsockAddress) Network() string { return "vsock" }
func (address vsockAddress) String() string  { return fmt.Sprintf("vsock:%d", address.port) }

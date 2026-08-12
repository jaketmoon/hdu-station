package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeResponseRejectsTrailingData(t *testing.T) {
	if _, err := decodeResponse(strings.NewReader("{\"version\":1}\nextra")); err == nil {
		t.Fatal("trailing response data was accepted")
	}
}

func TestProbeProtocolUsesUnixHalfClose(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "hdu-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	listener, err := net.Listen("unix", filepath.Join(root, "runtime.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		buffer := make([]byte, 256)
		_, _ = connection.Read(buffer)
		_, _ = connection.Write([]byte("{\"version\":1}\n"))
	}()
	connection, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("{\"version\":1,\"op\":\"ping\"}\n")); err != nil {
		t.Fatal(err)
	}
	if err := connection.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	result, err := decodeResponse(connection)
	if err != nil || result.Version != 1 {
		t.Fatalf("response = %#v, err = %v", result, err)
	}
}

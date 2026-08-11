//go:build darwin

package main

/*
#cgo LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"

// Wails 2.14 does not link UniformTypeIdentifiers when built with the Xcode 26
// SDK. Keep the workaround local to Darwin until upstream includes the flag.

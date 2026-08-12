package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type metadata struct {
	Platform  string `json:"platform"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
}

func main() {
	keyPath := flag.String("private-key", "", "PKCS#8 Ed25519 private key PEM file")
	digest := flag.String("sha256", "", "SHA-256 digest to sign as hexadecimal")
	platform := flag.String("platform", "", "target platform")
	file := flag.String("file", "", "artifact filename")
	output := flag.String("output", "", "metadata JSON output path")
	flag.Parse()

	if err := writeMetadata(*keyPath, *digest, *platform, *file, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeMetadata(keyPath, digest, platform, file, output string) error {
	if strings.TrimSpace(keyPath) == "" || strings.TrimSpace(digest) == "" || strings.TrimSpace(platform) == "" || strings.TrimSpace(file) == "" || strings.TrimSpace(output) == "" {
		return errors.New("private key, SHA-256 digest, platform, artifact file, and output path are required")
	}
	if filepath.Base(file) != file {
		return errors.New("artifact file must be a filename")
	}
	digestBytes, err := hex.DecodeString(strings.TrimSpace(digest))
	if err != nil || len(digestBytes) != 32 {
		return errors.New("SHA-256 digest must be 64 hexadecimal characters")
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read signing key: %w", err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return errors.New("signing key must be PEM encoded")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse signing key as PKCS#8: %w", err)
	}
	ed25519Key, ok := privateKey.(ed25519.PrivateKey)
	if !ok {
		return errors.New("signing key must be an Ed25519 private key")
	}
	value := metadata{
		Platform:  platform,
		File:      file,
		SHA256:    strings.ToLower(digest),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519Key, digestBytes)),
		PublicKey: base64.StdEncoding.EncodeToString(ed25519Key.Public().(ed25519.PublicKey)),
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode sandbox image metadata: %w", err)
	}
	directory := filepath.Dir(output)
	temporary, err := os.CreateTemp(directory, ".sandbox-image-metadata-*")
	if err != nil {
		return fmt.Errorf("create temporary metadata: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set metadata permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return fmt.Errorf("install metadata: %w", err)
	}
	return nil
}

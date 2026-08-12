package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteMetadataSignsDigestWithRawEd25519PublicKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	keyPath := filepath.Join(directory, "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey}), 0o600); err != nil {
		t.Fatal(err)
	}
	digestBytes := make([]byte, 32)
	for index := range digestBytes {
		digestBytes[index] = byte(index)
	}
	output := filepath.Join(directory, "image.json")
	if err := writeMetadata(keyPath, hex.EncodeToString(digestBytes), "darwin/arm64", "runtime.raw", output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var value metadata
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(value.Signature)
	if err != nil {
		t.Fatal(err)
	}
	storedPublicKey, err := base64.StdEncoding.DecodeString(value.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(storedPublicKey) != string(publicKey) || !ed25519.Verify(publicKey, digestBytes, signature) {
		t.Fatalf("metadata did not carry a verifiable raw Ed25519 public key: %#v", value)
	}
}

func TestWriteMetadataRejectsUnsafeArtifactFilename(t *testing.T) {
	if err := writeMetadata("key", "00", "darwin/arm64", "../runtime.raw", filepath.Join(t.TempDir(), "image.json")); err == nil {
		t.Fatal("unsafe filename was accepted")
	}
}

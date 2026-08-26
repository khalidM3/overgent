package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestEd25519RawEncodingSizes(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString(publicKey)); err != nil || len(decoded) != ed25519.PublicKeySize {
		t.Fatalf("public key encoding size=%d err=%v", len(decoded), err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString(privateKey)); err != nil || len(decoded) != ed25519.PrivateKeySize {
		t.Fatalf("private key encoding size=%d err=%v", len(decoded), err)
	}
}

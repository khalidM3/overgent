// Command release-keygen creates the offline Ed25519 trust anchor used to sign
// Overgent update metadata. It refuses to overwrite an existing private key.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	privateFile := flag.String("private-file", "", "new file that will receive the base64 private key")
	flag.Parse()
	if *privateFile == "" {
		fmt.Fprintln(os.Stderr, "private-file is required")
		os.Exit(2)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate update signing key:", err)
		os.Exit(1)
	}
	file, err := os.OpenFile(*privateFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintln(os.Stderr, "private key file already exists; refusing to overwrite it")
		} else {
			fmt.Fprintln(os.Stderr, "create private key file:", err)
		}
		os.Exit(1)
	}
	encodedPrivate := base64.StdEncoding.EncodeToString(privateKey) + "\n"
	if _, err = file.WriteString(encodedPrivate); err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, "write private key file:", err)
		os.Exit(1)
	}
	if err = file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close private key file:", err)
		os.Exit(1)
	}
	// The public key is intentionally the only stdout value so it can be copied
	// into the repository variable without exposing the private key.
	fmt.Println(base64.StdEncoding.EncodeToString(publicKey))
}

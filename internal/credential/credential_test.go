package credential

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRejectsInvalidCredentialInputsBeforePlatformAccess(t *testing.T) {
	ctx := context.Background()
	if err := Put(ctx, "", "secret"); err == nil {
		t.Fatal("empty account accepted")
	}
	if err := Put(ctx, "device\nother", "secret"); err == nil {
		t.Fatal("newline account accepted")
	}
	if _, err := Get(ctx, ""); err == nil {
		t.Fatal("empty lookup account accepted")
	}
}

func TestKeychainRoundTrip(t *testing.T) {
	if os.Getenv("OVERGENT_KEYCHAIN_LIVE") != "1" {
		t.Skip("set OVERGENT_KEYCHAIN_LIVE=1 for disposable Keychain validation")
	}
	account := fmt.Sprintf("validation-%d", time.Now().UnixNano())
	secret := "synthetic-overgent-validation-secret"
	ctx := context.Background()
	t.Cleanup(func() { _ = Delete(context.Background(), account) })
	if err := Put(ctx, account, secret); err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, account)
	if err != nil || got != secret {
		t.Fatalf("credential round trip length=%d err=%v", len(got), err)
	}
	if err := Delete(ctx, account); err != nil {
		t.Fatal(err)
	}
}

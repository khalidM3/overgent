package credential

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidationRejectsUnusableInputsBeforePlatformAccess(t *testing.T) {
	// These run on every host: the rules are pure, and a regression here would
	// otherwise only show up on the one platform whose store is reachable.
	cases := []struct {
		name    string
		account string
		secret  string
		putOK   bool
		lookOK  bool
	}{
		{name: "ordinary", account: "device-abc", secret: "s3cr3t", putOK: true, lookOK: true},
		{name: "empty account", account: "", secret: "s3cr3t"},
		{name: "empty secret", account: "device-abc", secret: "", lookOK: true},
		{name: "newline in account", account: "device\nother", secret: "s3cr3t"},
		{name: "carriage return in account", account: "device\rother", secret: "s3cr3t"},
		{name: "newline in secret", account: "device-abc", secret: "sec\nret", lookOK: true},
		{name: "carriage return in secret", account: "device-abc", secret: "sec\rret", lookOK: true},
		{name: "account with colon still valid", account: "backend:local", secret: "s3cr3t", putOK: true, lookOK: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validatePut(testCase.account, testCase.secret)
			if testCase.putOK != (err == nil) {
				t.Fatalf("validatePut ok=%v err=%v", testCase.putOK, err)
			}
			if err != nil && strings.Contains(err.Error(), testCase.secret) && testCase.secret != "" {
				t.Fatalf("validatePut error quoted the secret value: %v", err)
			}
			err = validateAccount(testCase.account)
			if testCase.lookOK != (err == nil) {
				t.Fatalf("validateAccount ok=%v err=%v", testCase.lookOK, err)
			}
		})
	}
}

func TestPublicSurfaceRejectsInvalidInputs(t *testing.T) {
	// The exported functions must reject before touching the platform, so this
	// stays green with no credential store present.
	ctx := context.Background()
	if err := Put(ctx, "", "secret"); err == nil {
		t.Fatal("empty account accepted")
	}
	if err := Put(ctx, "device\nother", "secret"); err == nil {
		t.Fatal("newline account accepted")
	}
	if err := Put(ctx, "device", ""); err == nil {
		t.Fatal("empty secret accepted")
	}
	if _, err := Get(ctx, ""); err == nil {
		t.Fatal("empty lookup account accepted")
	}
	if err := Delete(ctx, "device\rother"); err == nil {
		t.Fatal("carriage return account accepted for delete")
	}
}

func TestWindowsTargetName(t *testing.T) {
	// Pure string construction, so it is verifiable from any host. The format
	// is part of the on-disk contract: changing it strands every credential a
	// previous build wrote.
	cases := []struct {
		name    string
		account string
		want    string
	}{
		{name: "device id", account: "device-abc", want: "com.overgent.comice:device-abc"},
		{name: "backend instance", account: "local-backend-instance", want: "com.overgent.comice:local-backend-instance"},
		{name: "account containing a colon", account: "a:b", want: "com.overgent.comice:a:b"},
		{name: "unicode account", account: "café", want: "com.overgent.comice:café"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := windowsTargetName(testCase.account)
			if got != testCase.want {
				t.Fatalf("windowsTargetName(%q) = %q, want %q", testCase.account, got, testCase.want)
			}
			if !strings.HasPrefix(got, serviceName+":") {
				t.Fatalf("target name %q is not namespaced by the service constant", got)
			}
			if len(got) > windowsMaxTargetNameLength {
				t.Fatalf("target name is %d bytes, over the %d limit", len(got), windowsMaxTargetNameLength)
			}
		})
	}
}

func TestUnavailableErrorNamesTheRemedyAndForbidsPlaintext(t *testing.T) {
	err := error(&UnavailableError{
		Platform: "linux",
		Reason:   "there is no D-Bus session bus for this login",
		Remedy:   "start one with dbus-run-session",
		Cause:    errors.New("dial unix: no such file"),
	})
	message := err.Error()
	for _, want := range []string{"linux", "D-Bus session bus", "dbus-run-session", "plaintext", "no such file"} {
		if !strings.Contains(message, want) {
			t.Fatalf("UnavailableError message %q is missing %q", message, want)
		}
	}
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatal("UnavailableError is not recoverable with errors.As")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Fatal("UnavailableError dropped its cause")
	}
}

// liveStore reports whether this test run may exercise the member's real OS
// credential store, and skips with the reason when it may not.
//
// On macOS this stays behind the existing OVERGENT_KEYCHAIN_LIVE gate: the
// login Keychain is normally unlocked, so an ungated round trip would write
// and delete synthetic items on every `go test ./...` a contributor runs, and
// pop a GUI prompt when it is locked. On Linux and Windows the gate is
// reachability itself, probed below, so a native CI runner with a keyring or a
// Credential Manager gets real coverage with no extra configuration.
func liveStore(t *testing.T) {
	t.Helper()
	switch os.Getenv("OVERGENT_KEYCHAIN_LIVE") {
	case "1":
		return
	case "0":
		t.Skip("OVERGENT_KEYCHAIN_LIVE=0 disables live credential store tests")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("set OVERGENT_KEYCHAIN_LIVE=1 for disposable macOS Keychain validation")
	}
	probe := fmt.Sprintf("probe-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := Put(ctx, probe, "overgent-store-probe"); err != nil {
		t.Skipf("no OS credential store is reachable on this host: %v", err)
	}
	_ = Delete(ctx, probe)
}

func TestCredentialStoreRoundTrip(t *testing.T) {
	liveStore(t)
	account := fmt.Sprintf("validation-%d", time.Now().UnixNano())
	ctx := context.Background()
	t.Cleanup(func() { _ = Delete(context.Background(), account) })

	const (
		first  = "synthetic-overgent-validation-secret"
		second = "synthetic-overgent-rotated-secret-0123456789abcdef"
	)
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "put", run: func(t *testing.T) {
			if err := Put(ctx, account, first); err != nil {
				t.Fatalf("put: %v", err)
			}
		}},
		{name: "get", run: func(t *testing.T) {
			got, err := Get(ctx, account)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got != first {
				t.Fatalf("get returned a %d byte secret, want the %d byte one just stored", len(got), len(first))
			}
		}},
		{name: "overwrite", run: func(t *testing.T) {
			if err := Put(ctx, account, second); err != nil {
				t.Fatalf("overwrite: %v", err)
			}
			got, err := Get(ctx, account)
			if err != nil {
				t.Fatalf("get after overwrite: %v", err)
			}
			if got != second {
				t.Fatalf("get after overwrite returned a %d byte secret, want the %d byte replacement", len(got), len(second))
			}
		}},
		{name: "remove", run: func(t *testing.T) {
			if err := Delete(ctx, account); err != nil {
				t.Fatalf("remove: %v", err)
			}
		}},
		{name: "get after remove", run: func(t *testing.T) {
			got, err := Get(ctx, account)
			if err == nil {
				t.Fatalf("get after remove returned a %d byte secret", len(got))
			}
			// Linux and Windows classify this positively; macOS cannot, so the
			// weaker assertion above is the portable one.
			if runtime.GOOS != "darwin" && !errors.Is(err, ErrNotFound) {
				t.Fatalf("get after remove: want ErrNotFound, got %v", err)
			}
		}},
		{name: "remove after remove", run: func(t *testing.T) {
			if err := Delete(ctx, account); err == nil {
				t.Fatal("deleting an absent credential reported success")
			}
		}},
	}
	// The steps share one account and run in order: each depends on the state
	// the previous one left.
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.Fatalf("stopping after %q failed; later steps depend on it", step.name)
		}
	}
}

func TestRoundTripRejectsUnstorableSecretsWithoutTouchingTheStore(t *testing.T) {
	// Deliberately not gated: these must fail before any platform call, so a
	// host with no store still proves it.
	ctx := context.Background()
	account := fmt.Sprintf("validation-%d", time.Now().UnixNano())
	if err := Put(ctx, account, "line\nbreak"); err == nil {
		_ = Delete(ctx, account)
		t.Fatal("a secret containing a newline was accepted")
	}
}

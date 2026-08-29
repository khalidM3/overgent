package hosted

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifyCredentialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want CredentialStatus
	}{
		{"no error", nil, CredentialOK},
		{"revoked by an owner", &APIError{Status: 401, Code: "credential_revoked"}, CredentialRevoked},
		{"credential unknown to the deployment", &APIError{Status: 401, Code: "unauthorized"}, CredentialUnknown},
		{"unnamed 401 still means the credential failed", &APIError{Status: 401, Code: "unexpected_status"}, CredentialUnknown},
		{"wrapped errors are still classified", fmt.Errorf("open live Project: %w", &APIError{Status: 401, Code: "credential_revoked"}), CredentialRevoked},
		// Anything that is not an authentication failure must never trigger a
		// reset; losing the network is not losing your enrollment.
		{"offline", errors.New("call hosted API: connection refused"), CredentialUncertain},
		{"forbidden is authorization, not authentication", &APIError{Status: 403, Code: "forbidden"}, CredentialUncertain},
		{"server fault", &APIError{Status: 500, Code: "internal_error"}, CredentialUncertain},
		{"rate limited", &APIError{Status: 429, Code: "rate_limited"}, CredentialUncertain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyCredentialError(tc.err); got != tc.want {
				t.Fatalf("ClassifyCredentialError() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOnlyRejectionsAreRecoverable(t *testing.T) {
	if !CredentialRevoked.Recoverable() || !CredentialUnknown.Recoverable() {
		t.Fatal("a rejected credential must offer the member a reconnect")
	}
	if CredentialOK.Recoverable() || CredentialUncertain.Recoverable() {
		t.Fatal("a working or unverified credential must never offer to erase itself")
	}
}

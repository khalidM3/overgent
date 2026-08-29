package hosted

import "errors"

// CredentialStatus describes whether this device's stored credential is still
// accepted by the hosted API.
type CredentialStatus string

const (
	// CredentialOK means the credential authenticated successfully.
	CredentialOK CredentialStatus = "ok"
	// CredentialRevoked means a Project owner revoked this device. The member
	// cannot recover alone; they need a fresh invite.
	CredentialRevoked CredentialStatus = "revoked"
	// CredentialUnknown means the hosted API has no record of this credential.
	// It happens when the backing deployment was reset or restored, or when a
	// keychain entry outlived the account it belonged to.
	CredentialUnknown CredentialStatus = "unknown"
	// CredentialUncertain means the check could not complete - offline, timeout,
	// or a server fault. It must never be treated as a reason to reset anything.
	CredentialUncertain CredentialStatus = "uncertain"
)

// ClassifyCredentialError maps an error from any authenticated call onto a
// credential status. Both rejection codes arrive as HTTP 401, so the code - not
// the status - decides whether the member can recover on their own.
func ClassifyCredentialError(err error) CredentialStatus {
	if err == nil {
		return CredentialOK
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		// A transport failure says nothing about the credential.
		return CredentialUncertain
	}
	switch apiErr.Code {
	case "credential_revoked":
		return CredentialRevoked
	case "unauthorized":
		return CredentialUnknown
	}
	if apiErr.Status == 401 {
		return CredentialUnknown
	}
	return CredentialUncertain
}

// Recoverable reports whether a status means the local enrollment is dead and
// the member should be offered a reconnect.
func (s CredentialStatus) Recoverable() bool {
	return s == CredentialRevoked || s == CredentialUnknown
}

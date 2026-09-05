package onboarding

import (
	"errors"
	"strings"

	"github.com/khalidM3/overgent/internal/hosted"
)

// serverFieldError is the one sentence a member sees when the server they typed
// cannot be used, in the desktop's "connect to a different server" field and
// behind `--api` on the CLI. Two wordings for one rule is how a member ends up
// believing the app and the CLI accept different things.
const serverFieldError = "Enter an https:// server address with no path, for example https://api.example.com. Plain http is accepted only for 127.0.0.1."

// ValidateAPIOrigin canonicalizes a backend origin a member supplied.
//
// The rule is exactly hosted.New's, because that is the client every later call
// goes through: an HTTPS origin, or loopback HTTP for a backend running on this
// Mac, with no path, query, fragment, or userinfo. Checking it here means the
// member is told at the field rather than by the first request that fails.
func ValidateAPIOrigin(raw string) (string, error) {
	origin := strings.TrimRight(strings.TrimSpace(raw), "/")
	if origin == "" {
		return "", errors.New(serverFieldError)
	}
	if _, err := hosted.New(origin, ""); err != nil {
		return "", errors.New(serverFieldError)
	}
	return origin, nil
}

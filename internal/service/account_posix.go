//go:build darwin || linux

package service

import (
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"strconv"
)

// Account identifies the logged-in member a service is installed for.
//
// It is build tagged because the identity genuinely differs. macOS and Linux
// address the per-user manager by numeric uid - launchd's gui/<uid> domain and
// loginctl's user record both take one - so that is what the account carries
// here. Windows has no numeric uid at all, which is why account_windows.go
// exists rather than a runtime.GOOS branch at every call site.
type Account struct {
	Home string
	UID  int
}

// CurrentAccount resolves the account the calling process runs as.
func CurrentAccount() (Account, error) {
	account, err := user.Current()
	if err != nil {
		return Account{}, fmt.Errorf("resolve current user: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 || !filepath.IsAbs(account.HomeDir) {
		return Account{}, errors.New("current user has invalid home or uid")
	}
	return Account{Home: account.HomeDir, UID: uid}, nil
}

// NewManager binds a Manager to an account. It is the only construction site
// callers outside this package need, so adding a platform whose identity is not
// a uid does not touch them.
func NewManager(executable, configRoot string, account Account) Manager {
	return Manager{Executable: executable, ConfigRoot: configRoot, Home: account.Home, UID: account.UID}
}

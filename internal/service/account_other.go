//go:build !darwin && !linux && !windows

package service

import (
	"fmt"
	"os/user"
)

// Account identifies the logged-in member a service would be installed for.
// This platform has no qualified service manager, so the account carries only
// what every platform has: the home directory. Resolving it still succeeds so
// that "overgent service status" fails with the honest "not qualified on this
// platform", not with an account error that suggests the machine is misconfigured.
type Account struct {
	Home string
}

// CurrentAccount resolves the account the calling process runs as.
func CurrentAccount() (Account, error) {
	account, err := user.Current()
	if err != nil {
		return Account{}, fmt.Errorf("resolve current user: %w", err)
	}
	return Account{Home: account.HomeDir}, nil
}

// NewManager binds a Manager to an account.
func NewManager(executable, configRoot string, account Account) Manager {
	return Manager{Executable: executable, ConfigRoot: configRoot, Home: account.Home}
}

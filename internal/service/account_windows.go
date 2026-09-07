package service

import (
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
)

// Account identifies the logged-in member a service is installed for.
//
// On Windows user.Current().Uid is a SID string such as
// "S-1-5-21-1004336348-1177238915-682003330-512". Parsing it with strconv.Atoi
// - which is what the uid-shaped account does for launchd's gui/<uid> domain -
// fails for every user on the platform, so before this type existed every
// "overgent service" subcommand failed at account resolution and never reached
// the Manager at all. Task Scheduler addresses a user by name, so a name is
// what this account carries.
type Account struct {
	Home string
	User string
}

// CurrentAccount resolves the account the calling process runs as.
func CurrentAccount() (Account, error) {
	account, err := user.Current()
	if err != nil {
		return Account{}, fmt.Errorf("resolve current user: %w", err)
	}
	name := strings.TrimSpace(account.Username)
	if name == "" || !filepath.IsAbs(account.HomeDir) {
		return Account{}, errors.New("current user has invalid home or name")
	}
	return Account{Home: account.HomeDir, User: name}, nil
}

// NewManager binds a Manager to an account.
func NewManager(executable, configRoot string, account Account) Manager {
	return Manager{Executable: executable, ConfigRoot: configRoot, Home: account.Home, User: account.User}
}

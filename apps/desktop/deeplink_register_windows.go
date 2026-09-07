//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// registerDeepLinkScheme claims overgent:// for this build, per user.
//
// Windows resolves a scheme through HKEY_CLASSES_ROOT, which is a merged view of
// the machine-wide HKLM\Software\Classes and the calling user's
// HKCU\Software\Classes, with the user's view winning. Writing to HKCU is what
// makes this work for an ordinary member: HKLM needs administrator rights, and
// asking for elevation to open a link is not a trade worth making.
//
// It runs on every launch rather than in the installer because the command has
// to name where the executable is, and a member who moves the application or
// installs a second build would otherwise be left with a key pointing at a path
// that no longer runs anything.
func registerDeepLinkScheme() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	scheme := desktopURLScheme()

	root, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.SET_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		return fmt.Errorf("create the %s protocol key: %w", scheme, err)
	}
	defer root.Close()
	// The default value is the description shown in the "how do you want to
	// open this" dialog, and must start with "URL:" for the shell to treat the
	// key as a protocol at all.
	if err = root.SetStringValue("", "URL:"+desktopProductName()+" Protocol"); err != nil {
		return err
	}
	// An empty "URL Protocol" value is the flag itself: its presence is what
	// tells the shell this key describes a scheme rather than a file type.
	if err = root.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	icon, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme+`\DefaultIcon`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer icon.Close()
	if err = icon.SetStringValue("", windowsProtocolIcon(executable)); err != nil {
		return err
	}

	command, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer command.Close()
	return command.SetStringValue("", windowsProtocolCommand(executable))
}

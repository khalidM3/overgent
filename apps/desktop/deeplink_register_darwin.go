//go:build darwin

package main

// registerDeepLinkScheme is a no-op on macOS.
//
// CFBundleURLTypes in the Info.plist that scripts/build-desktop.mjs generates is
// the registration, and Launch Services reads it when the bundle is first seen
// and again whenever it changes on disk. There is nothing for a running process
// to write, and writing one would be worse than useless: a second claim on the
// scheme from inside the app is exactly how two builds start fighting over
// which one opens a link.
func registerDeepLinkScheme() error { return nil }

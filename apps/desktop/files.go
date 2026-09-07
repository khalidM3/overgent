package main

import (
	"os"
	"path/filepath"
)

// writeFileAtomically replaces a file in one step.
//
// The files this is used for - a desktop entry, an icon - are read by another
// process at a moment nothing here controls, so a partially written one is a
// real state a desktop environment can observe and cache. Writing beside the
// target and renaming over it means a reader sees either the previous file or
// the complete new one.
func writeFileAtomically(path string, body []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	if _, err = temporary.Write(body); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return err
	}
	if err = temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return err
	}
	if err = temporary.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err = os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

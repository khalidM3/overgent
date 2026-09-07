//go:build unix

package localbackend

import "syscall"

// newProcessGroupAttr puts the backend in its own process group so an
// interrupt aimed at the CLI or the development harness does not take it
// down; see the comment at the spawn call site.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

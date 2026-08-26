//go:build !darwin

package service

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("OS service management is not qualified on this platform")

type Manager struct {
	Executable, ConfigRoot, Home string
	UID                          int
}
type Status struct {
	Installed, Running bool
	Label              string
}

func (Manager) Install(context.Context) error          { return errUnsupported }
func (Manager) Start(context.Context) error            { return errUnsupported }
func (Manager) Stop(context.Context) error             { return errUnsupported }
func (Manager) Remove(context.Context) error           { return errUnsupported }
func (Manager) Status(context.Context) (Status, error) { return Status{}, errUnsupported }

//go:build !darwin

package service

import (
	"context"
	"errors"
	"io"
)

type unsupportedManager struct{}

func New(string, string) Manager                                    { return unsupportedManager{} }
func (unsupportedManager) Install(context.Context, io.Writer) error { return unsupported() }
func (unsupportedManager) Start(context.Context, io.Writer) error   { return unsupported() }
func (unsupportedManager) Status(context.Context, io.Writer) error  { return unsupported() }
func (unsupportedManager) Stop(context.Context, io.Writer) error    { return unsupported() }
func (unsupportedManager) Remove(context.Context, io.Writer) error  { return unsupported() }
func unsupported() error {
	return errors.New("user service lifecycle is unsupported by this Gate D spike on this platform")
}

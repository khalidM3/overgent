package service

import (
	"context"
	"io"
)

type Manager interface {
	Install(context.Context, io.Writer) error
	Start(context.Context, io.Writer) error
	Status(context.Context, io.Writer) error
	Stop(context.Context, io.Writer) error
	Remove(context.Context, io.Writer) error
}

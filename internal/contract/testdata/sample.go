package sample

import "context"

// Config carries the exported surface under test.
type Config struct {
	Name   string `json:"name"`
	hidden int
}

type Reader interface {
	Read(ctx context.Context, count int) ([]byte, error)
	Close() error
}

type Pair[T any] struct{ First T }

type Alias = Config

const MaxItems = 10

var Default = Config{}

func Rotate(ctx context.Context, key string) (string, error) { return "", nil }

func (c *Config) Apply(value int) error { return nil }

func unexported() {}

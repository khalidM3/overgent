package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRestartRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	for want := int64(1); want <= 2; want++ {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.RecordBoot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("boot count=%d want=%d", got, want)
		}
	}
}

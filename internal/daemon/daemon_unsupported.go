//go:build !darwin

package daemon

import (
	"context"
	"fmt"
	"net"
)

func acquire(string) (Lock, error) {
	return nil, fmt.Errorf("unsupported platform: local service validated only on macOS")
}
func serve(context.Context, string, Handler) error {
	return fmt.Errorf("unsupported platform: local service validated only on macOS")
}
func dial(context.Context, string) (net.Conn, error) {
	return nil, fmt.Errorf("unsupported platform: local service validated only on macOS")
}

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Health struct {
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	BootCount int64  `json:"bootCount"`
}

func Acquire(lockPath string) (io.Closer, error) {
	return acquireLock(lockPath)
}

func Serve(ctx context.Context, socketPath string, bootCount int64) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := listen(socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("protect socket: %w", err)
	}
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept IPC: %w", err)
		}
		go serveConn(conn, Health{Status: "ok", PID: os.Getpid(), BootCount: bootCount})
	}
}

func serveConn(conn net.Conn, health Health) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	var req struct {
		Method string `json:"method"`
	}
	if json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&req) != nil || req.Method != "health" {
		return
	}
	_ = json.NewEncoder(conn).Encode(health)
}

func Query(ctx context.Context, socketPath string, target any) error {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, network(), socketPath)
	if err != nil {
		return fmt.Errorf("connect current-user IPC: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(conn).Encode(map[string]string{"method": "health"}); err != nil {
		return fmt.Errorf("write health request: %w", err)
	}
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(target); err != nil {
		return fmt.Errorf("read health response: %w", err)
	}
	return nil
}

func Ping(ctx context.Context, socketPath string, out io.Writer) error {
	var health Health
	if err := Query(ctx, socketPath, &health); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(health)
}

func ensureParent(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create IPC directory: %w", err)
	}
	return nil
}

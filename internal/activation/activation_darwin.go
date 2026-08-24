//go:build darwin

package activation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

func Open(ctx context.Context, apiBaseURL, ticket string) error {
	base, err := url.Parse(strings.TrimRight(apiBaseURL, "/"))
	if err != nil || base.Host == "" || base.Path != "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "https" && !(base.Scheme == "http" && isLoopback(base.Hostname()))) {
		return errors.New("dashboard activation requires an HTTPS API origin or loopback validation origin")
	}
	if len(ticket) < 22 || len(ticket) > 512 || strings.ContainsAny(ticket, "\r\n") {
		return errors.New("dashboard activation ticket is invalid")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for dashboard activation handoff: %w", err)
	}
	defer listener.Close()
	nonceBytes := make([]byte, 16)
	if _, err = rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("generate dashboard activation nonce: %w", err)
	}
	path := "/activate/" + hex.EncodeToString(nonceBytes)
	action := strings.TrimRight(apiBaseURL, "/") + "/v1/dashboard-activations"
	served := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action "+base.Scheme+"://"+base.Host)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if writePage(w, action, ticket) == nil {
			select {
			case served <- struct{}{}:
			default:
			}
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	localURL := "http://" + listener.Addr().String() + path
	if err = exec.CommandContext(ctx, "/usr/bin/open", localURL).Run(); err != nil {
		_ = server.Close()
		return fmt.Errorf("open dashboard activation in browser: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case <-served:
	case <-waitCtx.Done():
		_ = server.Close()
		return fmt.Errorf("wait for browser activation handoff: %w", waitCtx.Err())
	}
	_ = server.Shutdown(context.Background())
	if serveErr := <-done; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve dashboard activation handoff: %w", serveErr)
	}
	return nil
}

func isLoopback(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

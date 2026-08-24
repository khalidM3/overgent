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
	"sync"
	"time"
)

type Handoff struct {
	url      string
	listener net.Listener
	server   *http.Server
	served   chan struct{}
	done     chan error
	once     sync.Once
}

func Start(apiBaseURL, ticket string) (*Handoff, error) {
	base, action, err := activationAction(apiBaseURL)
	if err != nil {
		return nil, err
	}
	if len(ticket) < 22 || len(ticket) > 512 || strings.ContainsAny(ticket, "\r\n") {
		return nil, errors.New("dashboard activation ticket is invalid")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for dashboard activation handoff: %w", err)
	}
	nonceBytes := make([]byte, 16)
	if _, err = rand.Read(nonceBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generate dashboard activation nonce: %w", err)
	}
	path := "/activate/" + hex.EncodeToString(nonceBytes)
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
	return &Handoff{url: "http://" + listener.Addr().String() + path, listener: listener, server: server, served: served, done: done}, nil
}

func activationAction(apiBaseURL string) (*url.URL, string, error) {
	base, err := url.Parse(strings.TrimRight(apiBaseURL, "/"))
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "https" && !(base.Scheme == "http" && isLoopback(base.Hostname()))) {
		return nil, "", errors.New("dashboard activation requires an HTTPS API origin or loopback validation origin")
	}
	if base.Path != "" && (!strings.HasPrefix(base.Path, "/") || strings.Contains(base.Path, "..")) {
		return nil, "", errors.New("dashboard activation origin path is invalid")
	}
	return base, strings.TrimRight(apiBaseURL, "/") + "/v1/dashboard-activations", nil
}

func (handoff *Handoff) URL() string { return handoff.url }

func (handoff *Handoff) Wait(ctx context.Context) error {
	defer handoff.close()
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case <-handoff.served:
	case <-waitCtx.Done():
		return fmt.Errorf("wait for browser activation handoff: %w", waitCtx.Err())
	}
	_ = handoff.server.Shutdown(context.Background())
	if serveErr := <-handoff.done; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve dashboard activation handoff: %w", serveErr)
	}
	return nil
}

func (handoff *Handoff) close() {
	handoff.once.Do(func() {
		_ = handoff.server.Close()
		_ = handoff.listener.Close()
	})
}

func Open(ctx context.Context, apiBaseURL, ticket string) error {
	handoff, err := Start(apiBaseURL, ticket)
	if err != nil {
		return err
	}
	if err = exec.CommandContext(ctx, "/usr/bin/open", handoff.URL()).Run(); err != nil {
		handoff.close()
		return fmt.Errorf("open dashboard activation in browser: %w", err)
	}
	return handoff.Wait(ctx)
}

func isLoopback(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

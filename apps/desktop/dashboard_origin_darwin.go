//go:build darwin

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// dashboardOrigin is the loopback origin a local-mode Project's dashboard is
// served from.
//
// The dashboard talks to its backend with same-origin relative requests
// (`/api/v1/...`), because browser activation sets a SameSite=Strict session
// cookie and a cookie is only sent back to the origin that set it. Hosted
// Overgent satisfies that with Vercel: the SPA and a `/v1` proxy on one origin
// (`api/v1/[...].js`, `vercel.json`). Development satisfies it with Vite, whose
// dev server serves the SPA and proxies `/api` to the loopback backend.
//
// Local mode has neither, so this is the third one: the same two jobs, on a
// loopback port, serving the dashboard the app already embeds and forwarding
// `/api/v1` to the bundled backend. Nothing here is a second implementation of
// the dashboard or of the API - it is the same asset bundle and a transparent
// forward.
type dashboardOrigin struct {
	origin   string
	listener net.Listener
	server   *http.Server

	mu     sync.RWMutex
	target *url.URL
}

// startDashboardOrigin binds a loopback port and serves assets immediately. The
// backend it forwards to is set later, because the backend's own ports are not
// known until the service has started it.
func startDashboardOrigin(assets fs.FS) (*dashboardOrigin, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for the local dashboard origin: %w", err)
	}
	handler := &dashboardOrigin{
		origin:   fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port),
		listener: listener,
	}
	handler.server = &http.Server{Handler: handler.routes(assets), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = handler.server.Serve(listener) }()
	return handler, nil
}

// apiPrefix is where this origin forwards to the backend. The embedded
// dashboard bundle is built to call `/api/v1/...`, and the proxy is mounted to
// match; anything that addresses the backend through this origin has to carry
// it too.
const apiPrefix = "/api"

// Origin is the base URL the dashboard itself is served from.
func (handler *dashboardOrigin) Origin() string { return handler.origin }

// ActivationOrigin is the base an activation ticket is posted against.
//
// It is Origin plus the API prefix, because internal/activation appends
// "/v1/dashboard-activations" to whatever it is given. Handing it the bare
// origin sent the ticket to "/v1/dashboard-activations", which this mux does
// not route to the proxy - it fell through to the SPA file server, which
// answered 200 with index.html. The form post therefore "succeeded", set no
// cookie, and the dashboard loaded with no session and told the member their
// browser had none. The proxy was covered by a test that spelled the prefix
// correctly; nothing covered the seam that produces it.
func (handler *dashboardOrigin) ActivationOrigin() string { return handler.origin + apiPrefix }

// SetBackend points the proxy at the loopback backend the service started.
func (handler *dashboardOrigin) SetBackend(siteOrigin string) error {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(siteOrigin), "/"))
	if err != nil || parsed.Host == "" || parsed.Path != "" {
		return errors.New("local dashboard backend must be an origin without a path")
	}
	if parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname()) {
		return errors.New("local dashboard backend must be a loopback HTTP origin")
	}
	handler.mu.Lock()
	handler.target = parsed
	handler.mu.Unlock()
	return nil
}

func (handler *dashboardOrigin) Close() error {
	if handler.server == nil {
		return nil
	}
	return handler.server.Close()
}

func (handler *dashboardOrigin) routes(assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(apiPrefix+"/v1/", http.HandlerFunc(handler.proxy))
	mux.Handle("/", spaFileServer(assets))
	return mux
}

// spaFileServer serves the embedded bundle, falling back to index.html so a
// deep link the SPA routes itself is not a 404 from the file server.
func spaFileServer(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		clean := path.Clean("/" + request.URL.Path)
		if clean != "/" {
			if file, err := assets.Open(strings.TrimPrefix(clean, "/")); err == nil {
				_ = file.Close()
				files.ServeHTTP(writer, request)
				return
			}
		}
		index, err := assets.Open("index.html")
		if err != nil {
			http.Error(writer, "dashboard assets are missing", http.StatusInternalServerError)
			return
		}
		defer index.Close()
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = io.Copy(writer, index)
	})
}

// proxy forwards one dashboard request to the bundled backend. It mirrors the
// hosted proxy in api/v1/[...].js: a fixed header allowlist in both directions,
// a bounded body, and manual redirect handling.
func (handler *dashboardOrigin) proxy(writer http.ResponseWriter, request *http.Request) {
	handler.mu.RLock()
	target := handler.target
	handler.mu.RUnlock()
	if target == nil {
		http.Error(writer, "the local backend is not running", http.StatusServiceUnavailable)
		return
	}
	upstream := *target
	upstream.Path = strings.TrimPrefix(request.URL.Path, apiPrefix)
	upstream.RawQuery = request.URL.RawQuery
	body := http.MaxBytesReader(writer, request.Body, 2<<20)
	forwarded, err := http.NewRequestWithContext(request.Context(), request.Method, upstream.String(), body)
	if err != nil {
		http.Error(writer, "dashboard request could not be forwarded", http.StatusBadGateway)
		return
	}
	for _, name := range []string{"Accept", "Authorization", "Content-Type", "Cookie", "User-Agent"} {
		if value := request.Header.Get(name); value != "" {
			forwarded.Header.Set(name, value)
		}
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		// A 303 from the activation route names where the *browser* should go
		// next, so it must reach the webview rather than being followed here.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(forwarded)
	if err != nil {
		http.Error(writer, "the local backend did not answer", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	for _, name := range []string{"Cache-Control", "Content-Disposition", "Content-Type", "Set-Cookie"} {
		for _, value := range response.Header.Values(name) {
			writer.Header().Add(name, value)
		}
	}
	if location := response.Header.Get("Location"); location != "" {
		writer.Header().Set("Location", localDashboardLocation(location))
	}
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, io.LimitReader(response.Body, 8<<20))
}

// localDashboardLocation keeps a redirect on this origin.
//
// The backend answers browser activation with a 303 whose target is the
// dashboard it expects to be served beside it: the hosted deployment's own
// `/dashboard?live=1`, or a relative `/?live=1` over loopback
// (convex/functions/http.ts). Older backends answered loopback with an
// absolute Vite URL on port 5173; both are the same intent - "now open the
// live view" - so any redirect to another origin is rewritten to this one's
// live route.
// Only the path is taken; the query and fragment the backend chose are kept.
func localDashboardLocation(location string) string {
	parsed, err := url.Parse(location)
	if err != nil {
		return "/?live=1"
	}
	if parsed.Host == "" && strings.HasPrefix(parsed.Path, "/") {
		// Already relative to this origin.
		return location
	}
	query := parsed.RawQuery
	if query == "" {
		query = "live=1"
	}
	return "/?" + query
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

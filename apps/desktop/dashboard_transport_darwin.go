//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/hosted"
)

// The native window stays on bundled assets. This is an allowlisted transport
// for existing /v1 dashboard operations, not a URL fetcher or a new HTTP server.
// Session secrets remain in Go memory and never enter the WebView.
type dashboardConnection struct {
	mu      sync.Mutex
	cookie  string
	expires time.Time
}
type DashboardReply struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

var dashboardRoutes = map[string]*regexp.Regexp{
	"GET":    regexp.MustCompile(`^/(dashboard/session|dashboard/projects/prj_[a-z0-9_]+|projects/prj_[a-z0-9_]+/(collaboration|members|access|export)|workstreams/wrk_[a-z0-9_]+/session-sharing)$`),
	"POST":   regexp.MustCompile(`^/(findings/[a-z0-9_]+/(feedback|state)|projects/prj_[a-z0-9_]+/(sync-cards|invites|invites/[a-z0-9_]+/revoke|members/[a-z0-9_]+/remove)|sync-cards/[a-z0-9_]+/(comments|resolve)|devices/dev_[a-z0-9_]+/revoke)$`),
	"PATCH":  regexp.MustCompile(`^/projects/prj_[a-z0-9_]+/member$`),
	"DELETE": regexp.MustCompile(`^/projects/prj_[a-z0-9_]+(/member)?$`),
}

func validDashboardRequest(method, path, body string) bool {
	rule := dashboardRoutes[method]
	return rule != nil && len(path) <= 512 && rule.MatchString(path) && len(body) <= 2<<20 && (body == "" || json.Valid([]byte(body))) && (method != "GET" || body == "")
}
func (service *OnboardingService) DashboardRequest(projectID, method, path, body string) (DashboardReply, error) {
	if !validDashboardRequest(method, path, body) {
		return DashboardReply{}, errors.New("unsupported Project operation")
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return DashboardReply{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return DashboardReply{}, err
	}
	backend, bound := cfg.BackendForProject(projectID)
	if !bound || backend.DeviceID == "" {
		return DashboardReply{}, errors.New("Project is not enrolled on this Mac")
	}
	// Validate the stored origin too; it is never provided by JavaScript.
	if _, err := hosted.New(backend.APIBaseURL, ""); err != nil {
		return DashboardReply{}, err
	}
	service.dashboardMu.Lock()
	if service.dashboardConnections == nil {
		service.dashboardConnections = map[string]*dashboardConnection{}
	}
	key := backend.ID + ":" + backend.DeviceID + ":" + projectID
	connection := service.dashboardConnections[key]
	if connection == nil {
		connection = &dashboardConnection{}
		service.dashboardConnections[key] = connection
	}
	service.dashboardMu.Unlock()
	connection.mu.Lock()
	defer connection.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if connection.cookie == "" || time.Now().After(connection.expires) {
		token, err := credential.Get(ctx, backend.DeviceID)
		if err != nil {
			return DashboardReply{}, errors.New("this Mac’s Project credential is unavailable")
		}
		api, err := hosted.New(backend.APIBaseURL, token)
		if err != nil {
			return DashboardReply{}, err
		}
		ticket, err := api.CreateDashboardTicket(ctx, projectID)
		if err != nil {
			return DashboardReply{}, errors.New("Project access could not be renewed")
		}
		payload, _ := json.Marshal(map[string]string{"ticket": ticket.Ticket})
		request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(backend.APIBaseURL, "/")+"/v1/dashboard-tickets/exchange", strings.NewReader(string(payload)))
		if err != nil {
			return DashboardReply{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return DashboardReply{}, errors.New("Project server did not answer")
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return DashboardReply{Status: response.StatusCode}, nil
		}
		for _, cookie := range response.Cookies() {
			if cookie.Name == "overgent_session" {
				connection.cookie = cookie.Value
			}
		}
		if connection.cookie == "" {
			return DashboardReply{}, errors.New("Project session was not created")
		}
		connection.expires = time.Now().Add(7 * time.Hour)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(backend.APIBaseURL, "/")+"/v1"+path, strings.NewReader(body))
	if err != nil {
		return DashboardReply{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "overgent_session", Value: connection.cookie})
	response, err := client.Do(request)
	if err != nil {
		return DashboardReply{}, errors.New("Project server did not answer")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return DashboardReply{}, errors.New("Project response is unavailable or too large")
	}
	if response.StatusCode == 401 {
		connection.cookie = ""
	}
	// Never forward HTML, redirects, headers or a raw server error to the bridge.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DashboardReply{Status: response.StatusCode}, nil
	}
	if len(data) != 0 && !json.Valid(data) {
		return DashboardReply{}, errors.New("Project server returned an unsupported response")
	}
	return DashboardReply{Status: response.StatusCode, Body: string(data)}, nil
}

// Export stays a deliberate save action. No credential or response header is
// returned to the WebView, and cancellation writes nothing.
func (service *OnboardingService) ExportProject(projectID string) error {
	reply, err := service.DashboardRequest(projectID, "GET", "/projects/"+projectID+"/export", "")
	if err != nil {
		return err
	}
	if reply.Status != 200 {
		return errors.New("Project data could not be exported")
	}
	destination, err := application.Get().Dialog.SaveFile().SetFilename("overgent-project.json").PromptForSingleSelection()
	if err != nil || destination == "" {
		return err
	}
	return os.WriteFile(destination, []byte(reply.Body), 0600)
}

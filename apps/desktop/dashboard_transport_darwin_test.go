//go:build darwin

package main

import (
	"github.com/khalidM3/overgent/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboardTransportRejectsUnboundedOrUnownedRoutes(t *testing.T) {
	for _, path := range []string{"https://example.com/v1/projects", "//example.com/", "/projects/prj_x/../../credentials", "/projects/prj_x/export?token=x", "/projects/prj_x/%2e%2e", "/dashboard-tickets/exchange", "/projects/prj_x/ai-settings", "/projects/prj_x/export#fragment"} {
		if validDashboardRequest("GET", path, "") {
			t.Fatalf("allowed path %q", path)
		}
	}
	if validDashboardRequest("POST", "/findings/fnd_x/state", strings.Repeat("x", 2<<20+1)) {
		t.Fatal("allowed oversized body")
	}
	if validDashboardRequest("POST", "/findings/fnd_x/state", "not JSON") {
		t.Fatal("allowed malformed body")
	}
	if !validDashboardRequest("POST", "/findings/fnd_x/state", `{"state":"dismissed"}`) {
		t.Fatal("rejected valid operation")
	}
	if !validDashboardRequest("GET", "/dashboard/projects/prj_x", "") {
		t.Fatal("rejected snapshot")
	}
}
func TestDashboardTransportKeepsCredentialsNativeAndRejectsRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cookie, err := r.Cookie("overgent_session")
		if err != nil || cookie.Value != "synthetic-session" {
			t.Error("missing native session")
		}
		w.Header().Set("Set-Cookie", "must-not-reach-javascript=synthetic")
		if calls == 2 {
			w.Header().Set("Location", "http://127.0.0.1:1/leak")
			w.WriteHeader(302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"members":[]}`))
	}))
	defer server.Close()
	paths, _ := config.Resolve(t.TempDir())
	cfg := config.Single(server.URL, "dev_synthetic", []config.Workspace{{ID: "wsp_test", ProjectID: "prj_test", Root: t.TempDir()}})
	if err := config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	backend, _ := cfg.BackendForProject("prj_test")
	service := &OnboardingService{configRoot: paths.Root, dashboardConnections: map[string]*dashboardConnection{backend.ID + ":dev_synthetic:prj_test": {cookie: "synthetic-session", expires: time.Now().Add(time.Hour)}}}
	reply, err := service.DashboardRequest("prj_test", "GET", "/projects/prj_test/members", "")
	if err != nil || reply.Status != 200 || reply.Body != `{"members":[]}` {
		t.Fatalf("request failed: %#v %v", reply, err)
	}
	reply, err = service.DashboardRequest("prj_test", "GET", "/projects/prj_test/members", "")
	if err != nil || reply.Status != 302 || reply.Body != "" || calls != 2 {
		t.Fatalf("redirect followed: %#v %v", reply, err)
	}
	if _, err = service.DashboardRequest("prj_unknown", "GET", "/projects/prj_test/members", ""); err == nil {
		t.Fatal("unknown Project reached transport")
	}
}

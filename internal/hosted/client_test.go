package hosted

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesVersionedContractAndBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %#v", r.URL.Path, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["label"] != "Fixture" || body["deviceLabel"] != "Device" {
			t.Fatalf("request body=%#v err=%v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"prj_fixture","label":"Fixture"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	project, err := client.CreateProject(context.Background(), "Fixture", "Device")
	if err != nil || project.ID != "prj_fixture" {
		t.Fatalf("project=%#v err=%v", project, err)
	}
}

func TestClientRejectsInsecureRemoteAndReturnsStableAPIError(t *testing.T) {
	if _, err := New("http://example.com", "token"); err == nil {
		t.Fatal("insecure remote API accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"credential_revoked","requestId":"req_fixture","retryable":false,"message":"redacted"}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Bootstrap(context.Background())
	failure, ok := err.(*APIError)
	if !ok || failure.Code != "credential_revoked" || failure.RequestID != "req_fixture" || failure.Retryable {
		t.Fatalf("API error=%#v", err)
	}
}

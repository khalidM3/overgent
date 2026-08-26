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
		// An unchosen display name must be omitted rather than sent as the
		// device label, so the hosted side knows the member still owes a choice.
		if _, present := body["displayName"]; present {
			t.Fatalf("unchosen display name must not be sent: %#v", body)
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
	project, err := client.CreateProject(context.Background(), "Fixture", "Device", "", "stickguy/test")
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

func TestCreateBriefUsesFrozenAuthenticatedContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/workstreams/wrk_fixture/briefs" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatalf("unexpected request: %s %s %#v", r.Method, r.URL.Path, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["trigger"] != "checkpoint" || body["sinceCursor"] != "cur_fixture" || body["approximateTokenBudget"] != float64(400) {
			t.Fatalf("request body=%#v err=%v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"briefId":"brf_fixture","projectId":"prj_fixture","repositoryId":"rep_fixture","workstreamId":"wrk_fixture","generatedAt":"2026-08-23T00:00:00Z","trigger":"checkpoint","nextCursor":"cur_next","contextRevision":3,"requestedBudget":400,"renderedSize":120,"truncated":false,"items":[{"id":"itm_fixture","kind":"finding","text":"Coordinate the contract","relevanceReason":"same interface","fidelity":"structural","advisoryAction":"inspect","revision":2,"priority":1}]}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	brief, err := client.CreateBrief(context.Background(), "wrk_fixture", "checkpoint", "cur_fixture", 400)
	if err != nil || brief.BriefID != "brf_fixture" || brief.NextCursor != "cur_next" || len(brief.Items) != 1 || brief.Items[0].ID != "itm_fixture" {
		t.Fatalf("brief=%#v err=%v", brief, err)
	}
}

func TestClientSendsChosenDisplayNameSeparatelyFromDeviceLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["deviceLabel"] != "Khalid's MacBook" || body["displayName"] != "Khalid M" {
			t.Fatalf("device label and display name must stay distinct: %#v", body)
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
	if _, err := client.CreateProject(context.Background(), "Fixture", "Khalid's MacBook", "Khalid M", "stickguy/test"); err != nil {
		t.Fatal(err)
	}
}

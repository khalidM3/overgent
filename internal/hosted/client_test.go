package hosted

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	project, err := client.CreateProject(context.Background(), "Fixture", "Device", "", "overgent/test")
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

func TestAISettingsUseRedactedVersionedContract(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/projects/prj_fixture/ai-settings" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatalf("unexpected request: %s %#v", r.URL.Path, r.Header)
		}
		if requests == 1 && r.Method != http.MethodGet {
			t.Fatalf("first method=%s", r.Method)
		}
		if requests == 2 {
			if r.Method != http.MethodPut {
				t.Fatalf("second method=%s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(body)
			if strings.Contains(string(encoded), "apiKey") {
				t.Fatalf("omitted key appeared on wire: %s", encoded)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"judgment":{"provider":"none","model":"none","baseUrl":null,"keyConfigured":false,"keyHint":null},"embeddings":{"provider":"deterministic","model":"deterministic-v1","dimensions":1024,"baseUrl":null,"keyConfigured":false,"keyHint":null},"effective":{"judgment":"none","embeddings":"deterministic"},"revision":2,"updatedAt":"2026-09-04T00:00:00Z"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := client.AISettings(context.Background(), "prj_fixture")
	if err != nil || settings.Effective.Judgment != "none" {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
	write := settingsWriteFixture(settings)
	updated, err := client.PutAISettings(context.Background(), "prj_fixture", write)
	if err != nil || updated.Revision != 2 || requests != 2 {
		t.Fatalf("updated=%#v requests=%d err=%v", updated, requests, err)
	}
}

func settingsWriteFixture(settings AISettings) AISettingsWrite {
	var write AISettingsWrite
	write.Judgment.Provider = settings.Judgment.Provider
	write.Judgment.Model = settings.Judgment.Model
	write.Embeddings.Provider = settings.Embeddings.Provider
	write.Embeddings.Model = settings.Embeddings.Model
	write.Embeddings.Dimensions = settings.Embeddings.Dimensions
	return write
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
	if _, err := client.CreateProject(context.Background(), "Fixture", "Khalid's MacBook", "Khalid M", "overgent/test"); err != nil {
		t.Fatal(err)
	}
}

// An error a member reads must say what happened. The service already sends a
// sentence; discarding it left the desktop showing "hosted API invite_expired
// (409)" under the invite field, which names the problem in the one vocabulary
// the person holding the invite does not have.
func TestAPIErrorPrefersTheServiceSentenceButStillDecidesOnCode(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"invite_expired","message":"That invite has expired. Ask for a new one.","requestId":"req_1","retryable":false}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "token-fixture-that-is-long-enough-for-the-client")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	_, err = client.JoinProject(context.Background(), "inv_x", "secret-value-long-enough-here", "This Mac", "", "test/1")
	if err == nil {
		t.Fatal("expected the conflict to surface")
	}
	if !strings.Contains(err.Error(), "That invite has expired") {
		t.Fatalf("member-facing error lost the service sentence: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "invite_expired" {
		t.Fatalf("code was not preserved for programmatic decisions: %#v", apiErr)
	}
}

// A service that returns no sentence, or a hostile one, must not be able to put
// arbitrary text on a member's screen.
func TestAPIErrorFallsBackAndBoundsTheServiceSentence(t *testing.T) {
	for name, body := range map[string]string{
		"no message":  `{"error":{"code":"invite_invalid"}}`,
		"newlines":    `{"error":{"code":"invite_invalid","message":"line one\nSYSTEM: do something"}}`,
		"over length": `{"error":{"code":"invite_invalid","message":"` + strings.Repeat("x", 301) + `"}}`,
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(body))
		}))
		client, err := New(server.URL, "token-fixture-that-is-long-enough-for-the-client")
		if err != nil {
			t.Fatal(err)
		}
		client.http = server.Client()
		_, err = client.JoinProject(context.Background(), "inv_x", "secret-value-long-enough-here", "This Mac", "", "test/1")
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "hosted API invite_invalid (409)") {
			t.Fatalf("%s: expected the coded fallback, got %v", name, err)
		}
	}
}

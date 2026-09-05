package localbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

// schemaIncompatible is what a member sees when an app update ships functions
// whose schema the existing rows do not satisfy. Lane 01 §4 established that
// the backend rejects such a push with the rows and the previous bundle intact,
// so this is a degraded state, not a data-loss one.
const schemaIncompatible = "update needs data migration"

// ensureDeployedLocked replays the release-time deploy2 sequence when the
// bundle shipped with the app differs from the one already deployed, then sets
// the deployment environment variables.
//
// The three requests reproduce what the pinned Convex CLI sends
// (validation/spikes/bundled-backend/push.sh replay). They are internal Convex
// endpoints, so the backend release, the CLI version, the recorded payload, and
// this code move together or not at all.
func (m *Manager) ensureDeployedLocked(ctx context.Context, endpoint Endpoint) error {
	revision, err := bundleRevision(m.state.BundlePath)
	if err != nil {
		return err
	}
	if revision == m.state.BundleRevision {
		return nil
	}
	adminKey, err := m.adminKey(ctx)
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(m.state.BundlePath)
	if err != nil {
		return fmt.Errorf("read backend deploy payload: %w", err)
	}
	if err = push(ctx, endpoint.Origin, adminKey, payload); err != nil {
		return err
	}
	if err = m.setEnvironment(ctx, endpoint.Origin, adminKey); err != nil {
		return err
	}
	state := m.state
	state.BundleRevision = revision
	state.Version = backendRelease
	m.state = state
	return m.save(state)
}

// adminKey asks the backend binary itself to derive the key from the instance
// name and secret, exactly as the Convex CLI does. Deriving it here would pin a
// second internal detail for no benefit.
func (m *Manager) adminKey(ctx context.Context) (string, error) {
	secret, err := m.instanceSecret(ctx, m.state.InstanceName)
	if err != nil {
		return "", err
	}
	output, err := exec.CommandContext(ctx, m.state.BinaryPath,
		"keygen", "admin-key",
		"--instance-name", m.state.InstanceName,
		"--instance-secret", secret,
	).Output()
	if err != nil {
		return "", errors.New("derive local backend admin key: backend keygen failed")
	}
	key := strings.TrimSpace(string(output))
	if key == "" || !strings.Contains(key, "|") {
		return "", errors.New("local backend produced an admin key in an unexpected form")
	}
	return key, nil
}

func push(ctx context.Context, origin, adminKey string, payload []byte) error {
	start, err := withAdminKey(payload, adminKey)
	if err != nil {
		return err
	}
	raw, err := request(ctx, origin+"/api/deploy2/start_push", adminKey, start, true)
	if err != nil {
		return fmt.Errorf("start backend push: %w", err)
	}
	started, err := normalize(raw)
	if err != nil {
		return fmt.Errorf("decode backend push response: %w", err)
	}
	if err = waitForSchema(ctx, origin, adminKey, started); err != nil {
		return err
	}
	finish, err := json.Marshal(map[string]any{"adminKey": adminKey, "startPush": started, "dryRun": false})
	if err != nil {
		return fmt.Errorf("encode backend finish push: %w", err)
	}
	if _, err = request(ctx, origin+"/api/deploy2/finish_push", adminKey, finish, true); err != nil {
		return fmt.Errorf("finish backend push: %w", err)
	}
	return nil
}

// withAdminKey substitutes this install's key into the release-time payload,
// which ships with a placeholder so no key is ever committed or distributed.
func withAdminKey(payload []byte, adminKey string) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("decode backend deploy payload: %w", err)
	}
	encoded, err := json.Marshal(adminKey)
	if err != nil {
		return nil, err
	}
	document["adminKey"] = encoded
	return json.Marshal(document)
}

func waitForSchema(ctx context.Context, origin, adminKey string, started map[string]any) error {
	change, ok := started["schemaChange"]
	if !ok {
		return errors.New("backend push response carried no schema change")
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		body, err := json.Marshal(map[string]any{
			"adminKey": adminKey, "schemaChange": change, "timeoutMs": 10000, "dryRun": false,
		})
		if err != nil {
			return err
		}
		answer, err := request(ctx, origin+"/api/deploy2/wait_for_schema", adminKey, body, false)
		if err != nil {
			return fmt.Errorf("wait for backend schema: %w", err)
		}
		var state struct{ Type string }
		if err = json.Unmarshal(answer, &state); err != nil {
			return fmt.Errorf("decode backend schema state: %w", err)
		}
		switch state.Type {
		case "complete":
			return nil
		case "inProgress":
			continue
		default:
			// The backend refused the schema against existing rows. It keeps
			// both the rows and the previous bundle, so the honest report is
			// that the update did not happen (Lane 01 §4).
			return errors.New(schemaIncompatible)
		}
	}
	return errors.New("backend schema change did not complete")
}

// setEnvironment writes the deployment secrets the functions read. Values are
// sent once, over loopback, and never logged; OVERGENT_SECRETS_KEY is generated
// per install and lives in the Keychain, not in any file.
func (m *Manager) setEnvironment(ctx context.Context, origin, adminKey string) error {
	key, err := m.secretsKey(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"changes": []map[string]string{{"name": "OVERGENT_SECRETS_KEY", "value": key}},
	})
	if err != nil {
		return err
	}
	if _, err = request(ctx, origin+"/api/update_environment_variables", adminKey, body, false); err != nil {
		return errors.New("set local backend environment: request rejected")
	}
	return nil
}

func (m *Manager) secretsKey(ctx context.Context) (string, error) {
	if m.creds == nil {
		return "", errors.New("local backend requires a credential store")
	}
	if key, err := m.creds.Get(ctx, secretsKeyAccount); err == nil && key != "" {
		return key, nil
	}
	key, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err = m.creds.Put(ctx, secretsKeyAccount, key); err != nil {
		return "", fmt.Errorf("store backend secrets key: %w", err)
	}
	return key, nil
}

// request performs one pinned deploy2 call. Errors never carry the response
// body verbatim: it echoes the admin key back on some paths.
func request(ctx context.Context, url, adminKey string, body []byte, compress bool) ([]byte, error) {
	payload := body
	if compress {
		var buffer bytes.Buffer
		writer := brotli.NewWriterLevel(&buffer, 4)
		if _, err := writer.Write(body); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		payload = buffer.Bytes()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Convex "+adminKey)
	httpRequest.Header.Set("Convex-Client", convexClientVersion)
	httpRequest.Header.Set("Content-Type", "application/json")
	if compress {
		httpRequest.Header.Set("Content-Encoding", "br")
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, errors.New("local backend did not answer")
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local backend returned HTTP %d", response.StatusCode)
	}
	return answer, nil
}

// normalize re-encodes a backend response the way the Convex CLI's JSON.parse
// and the spike's jq do.
//
// This is not cosmetic. The backend's start_push response contains objects with
// a repeated "type" key - a serde tagging artifact - and its own deserializer
// then rejects a body that still has both. JavaScript and jq keep only the last
// value, so the CLI and the spike's shell replay never saw it; passing the
// bytes straight back through as a json.RawMessage did, and wait_for_schema
// answered HTTP 400 "unknown variant `SerializedDeveloperIndexConfig`".
// Decoding into a map applies the same last-one-wins rule. UseNumber keeps
// integers exact instead of routing them through float64.
func normalize(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	return document, nil
}

//go:build darwin

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/overgent/overgent/internal/app"
	"github.com/overgent/overgent/internal/claudesetup"
	"github.com/overgent/overgent/internal/codexsetup"
	"github.com/overgent/overgent/internal/config"
	"github.com/overgent/overgent/internal/daemon"
	"github.com/overgent/overgent/internal/hosted"
)

func TestRealCodexAndClaudeLifecycle(t *testing.T) {
	if os.Getenv("OVERGENT_REAL_CLIENT_SMOKE") != "1" {
		t.Skip("set OVERGENT_REAL_CLIENT_SMOKE=1 for credentialed real-client smoke")
	}
	overgent := requiredExecutable(t, "OVERGENT_BINARY")
	codex := requiredExecutable(t, "CODEX_BINARY")
	claude := requiredExecutable(t, "CLAUDE_BINARY")
	only := os.Getenv("OVERGENT_REAL_CLIENT_ONLY")
	if only != "" && only != "codex" && only != "claude" {
		t.Fatalf("OVERGENT_REAL_CLIENT_ONLY must be codex or claude")
	}
	root, err := os.MkdirTemp("/private/tmp", "overgent-l5-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	project, state := filepath.Join(root, "project"), filepath.Join(root, "state")
	if err = os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	runFixture(t, project, "git", "init", "-q")
	runFixture(t, project, "git", "config", "user.email", "fixture@overgent.com")
	runFixture(t, project, "git", "config", "user.name", "Overgent Fixture")
	if err = os.WriteFile(filepath.Join(project, "README.md"), []byte("synthetic fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFixture(t, project, "git", "add", "README.md")
	runFixture(t, project, "git", "commit", "-qm", "fixture")
	baseline := strings.TrimSpace(runFixture(t, project, "git", "rev-parse", "HEAD"))
	paths, err := config.Resolve(state)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Version: 1, DeviceID: "dev_fixture", Workspaces: []config.Workspace{{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: project, Baseline: baseline, Fingerprint: "synthetic_fixture"}}}
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = (codexsetup.Manager{ProjectRoot: project, ConfigRoot: state, Executable: overgent}).Setup(); err != nil {
		t.Fatal(err)
	}
	if _, err = (claudesetup.Manager{ProjectRoot: project, ConfigRoot: state, Executable: overgent}).Setup(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	sender := &realClientFixtureSender{}
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, state, sender) }()
	waitForService(t, paths.Socket)
	preflightProductionBridge(t, ctx, overgent, state, project)

	prompt := `Use only the Overgent MCP tools. Do not inspect or modify files, run commands, or stop after the initial brief. Execute every call below in order even when a prior result is degraded, then return only done.
1. begin_work: workspace_id wsp_fixture, idempotency_key begin_CLIENT_1, title Bounded lifecycle, outcome Prove the complete lifecycle.
2. update_intent: workspace_id wsp_fixture, idempotency_key update_CLIENT_1, revision 1, title Refined lifecycle, outcome Prove every lifecycle call.
3. check_coordination: workspace_id wsp_fixture, trigger before_broad_edit, approximate_token_budget 400.
4. report_checkpoint: workspace_id wsp_fixture, checkpoint_id chk_CLIENT_1, summary Bounded checkpoint passed, with one verification item: state passed, check_kind test, label Lifecycle fixture, summary Passed.
5. Repeat step 4 with exactly the same arguments.
6. acknowledge_context: workspace_id wsp_fixture, idempotency_key ack_CLIENT_1, brief_id brf_fixture, considered_item_ids containing itm_fixture.
7. report_event: workspace_id wsp_fixture, idempotency_key event_CLIENT_1, kind decision, summary Retain the bounded MCP lifecycle.
8. finish_work: workspace_id wsp_fixture, idempotency_key finish_CLIENT_1, outcome Lifecycle delivered, summary Final bounded verification, with one verification item: state passed, check_kind test, label Final lifecycle fixture, summary Passed.`
	expectedRecords, expectedBriefs := 1, 2
	if only != "claude" {
		codexPrompt := strings.ReplaceAll(prompt, "CLIENT", "codex")
		trust := fmt.Sprintf("projects={ %q = { trust_level = \"trusted\" } }", project)
		codexOutput := runClient(t, codexPrompt, codex, "exec", "--ephemeral", "--json", "--sandbox", "workspace-write", "--add-dir", state, "--ignore-user-config", "--ignore-rules", "-C", project, "-c", trust, "-")
		assertLifecycleCalls(t, "codex", codexOutput)
		expectedRecords, expectedBriefs = expectedRecords+6, expectedBriefs+4
		if count := idempotencyCount(t, paths.DB); count < expectedRecords {
			t.Fatalf("durable lifecycle idempotency records=%d, want >=%d; redacted tool diagnostics=%v", count, expectedRecords, toolDiagnostics(codexOutput))
		}
	}

	if only != "codex" {
		claudePrompt := strings.ReplaceAll(prompt, "CLIENT", "claude")
		claudeOutput := runClientInDir(t, project, claudePrompt, claude, "-p", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", filepath.Join(project, ".mcp.json"), "--permission-mode", "dontAsk", "--allowedTools", "mcp__overgent__*", "--tools", "", "--max-budget-usd", "1")
		assertLifecycleCalls(t, "claude", claudeOutput)
		expectedRecords, expectedBriefs = expectedRecords+6, expectedBriefs+4
		if count := idempotencyCount(t, paths.DB); count < expectedRecords {
			t.Fatalf("durable lifecycle idempotency records=%d, want >=%d; redacted tool diagnostics=%v", count, expectedRecords, toolDiagnostics(claudeOutput))
		}
	}

	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not stop")
	}
	if sender.BriefCalls() < expectedBriefs {
		t.Fatalf("coordination brief calls=%d, want at least %d", sender.BriefCalls(), expectedBriefs)
	}
}

func preflightProductionBridge(t *testing.T, ctx context.Context, binary, state, project string) {
	t.Helper()
	command := exec.Command(binary, "--config-root", state, "mcp")
	command.Dir = project
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "real-client-preflight", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("production bridge preflight connect: %v", err)
	}
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "check_coordination", Arguments: map[string]any{"workspace_id": "wsp_fixture", "trigger": "before_broad_edit", "approximate_token_budget": 400}})
	if err == nil && result != nil && !result.IsError {
		result, err = session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "begin_work", Arguments: map[string]any{"workspace_id": "wsp_fixture", "idempotency_key": "preflight_1", "title": "Production bridge preflight", "outcome": "Prove durable local mutation"}})
	}
	if closeErr := session.Close(); err == nil {
		err = closeErr
	}
	if err != nil || result == nil || result.IsError {
		t.Fatalf("production bridge preflight result_error=%t err=%v", result != nil && result.IsError, err)
	}
}

type realClientFixtureSender struct {
	mu    sync.Mutex
	brief int
}

func (*realClientFixtureSender) Send(context.Context, string, []byte) error {
	return errors.New("synthetic fixture has no egress")
}
func (s *realClientFixtureSender) CreateBrief(_ context.Context, workstreamID, trigger, _ string, budget int) (hosted.CoordinationBrief, error) {
	s.mu.Lock()
	s.brief++
	s.mu.Unlock()
	return hosted.CoordinationBrief{BriefID: "brf_fixture", ProjectID: "prj_fixture", RepositoryID: "rep_fixture", WorkstreamID: workstreamID, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Trigger: trigger, ContextRevision: 1, RequestedBudget: budget, RenderedSize: 64, Items: []hosted.BriefItem{{ID: "itm_fixture", Kind: "decision", Text: "Coordinate the bounded fixture lifecycle.", RelevanceReason: "same workstream", Fidelity: "structural", AdvisoryAction: "acknowledge", Revision: 1, Priority: 1}}}, nil
}
func (s *realClientFixtureSender) BriefCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.brief
}

func requiredExecutable(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func runFixture(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture command %s failed: %v", name, err)
	}
	return string(output)
}

func waitForService(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := daemon.Call(context.Background(), socket, daemon.Request{Method: "health"})
		if err == nil && response.OK {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("service did not become healthy")
}

func runClient(t *testing.T, prompt, name string, args ...string) []byte {
	t.Helper()
	return runClientInDir(t, "", prompt, name, args...)
}

func runClientInDir(t *testing.T, dir, prompt, name string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Stdin = strings.NewReader(prompt)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		categories := clientFailureCategories(output.Bytes())
		if !containsString(categories, "result_success") {
			t.Fatalf("real client %s failed without retaining raw output: %v (output bytes=%d; redacted categories=%v)", filepath.Base(name), err, output.Len(), categories)
		}
	}
	return output.Bytes()
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func clientFailureCategories(output []byte) []string {
	set := map[string]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event["type"] != "result" {
			continue
		}
		if subtype, _ := event["subtype"].(string); subtype != "" {
			set["result_"+subtype] = struct{}{}
		}
		encoded, _ := json.Marshal([]any{event["result"], event["errors"]})
		for _, category := range classifyClientText(string(encoded)) {
			set[category] = struct{}{}
		}
	}
	if len(set) == 0 {
		for _, category := range classifyClientText(string(output)) {
			set[category] = struct{}{}
		}
	}
	categories := make([]string, 0, len(set))
	for category := range set {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}

func classifyClientText(raw string) []string {
	value := strings.ToLower(raw)
	patterns := []struct{ category, pattern string }{
		{"authentication", "authentication"}, {"authentication", "not logged"}, {"authentication", "api key"},
		{"permission", "permission"}, {"configuration", "config"}, {"invalid_input", "invalid"},
		{"unknown_option", "unknown option"}, {"budget", "budget"}, {"network", "network"},
		{"connection", "connection"}, {"model", "model"}, {"rate_limit", "rate limit"},
		{"credit", "credit"}, {"oauth", "oauth"}, {"mcp", "mcp"},
	}
	set := map[string]struct{}{}
	for _, candidate := range patterns {
		if strings.Contains(value, candidate.pattern) {
			set[candidate.category] = struct{}{}
		}
	}
	categories := make([]string, 0, len(set))
	for category := range set {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}

func assertLifecycleCalls(t *testing.T, client string, output []byte) {
	t.Helper()
	calls := map[string]map[string]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var value any
		if json.Unmarshal(scanner.Bytes(), &value) == nil {
			collectToolCalls(value, calls)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("parse %s metadata: %v", client, err)
	}
	want := []string{"begin_work", "update_intent", "check_coordination", "report_checkpoint", "acknowledge_context", "report_event", "finish_work"}
	for _, name := range want {
		minimum := 1
		if name == "report_checkpoint" {
			minimum = 2
		}
		if len(calls[name]) < minimum {
			t.Fatalf("%s lifecycle metadata counts=%v; %s calls=%d, want >=%d", client, sortedCounts(calls), name, len(calls[name]), minimum)
		}
	}
	t.Logf("%s lifecycle tool counts=%v", client, sortedCounts(calls))
}

func collectToolCalls(value any, calls map[string]map[string]struct{}) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectToolCalls(item, calls)
		}
	case map[string]any:
		name := ""
		if server, _ := typed["server"].(string); server == "overgent" {
			if tool, _ := typed["tool"].(string); tool != "" {
				name = tool
			}
		}
		if qualified, _ := typed["name"].(string); strings.HasPrefix(qualified, "mcp__overgent__") {
			name = strings.TrimPrefix(qualified, "mcp__overgent__")
		}
		if id, _ := typed["id"].(string); name != "" && id != "" {
			if calls[name] == nil {
				calls[name] = map[string]struct{}{}
			}
			calls[name][id] = struct{}{}
		}
		for _, item := range typed {
			collectToolCalls(item, calls)
		}
	}
}

func sortedCounts(calls map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(calls))
	for name, ids := range calls {
		out = append(out, fmt.Sprintf("%s=%d", name, len(ids)))
	}
	sort.Strings(out)
	return out
}

func idempotencyCount(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM idempotency_keys`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func toolDiagnostics(output []byte) []string {
	var diagnostics []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var value any
		if json.Unmarshal(scanner.Bytes(), &value) == nil {
			collectDiagnostics(value, &diagnostics)
		}
	}
	sort.Strings(diagnostics)
	return diagnostics
}

func collectDiagnostics(value any, diagnostics *[]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectDiagnostics(item, diagnostics)
		}
	case map[string]any:
		tool := ""
		if server, _ := typed["server"].(string); server == "overgent" {
			tool, _ = typed["tool"].(string)
		}
		if qualified, _ := typed["name"].(string); strings.HasPrefix(qualified, "mcp__overgent__") {
			tool = strings.TrimPrefix(qualified, "mcp__overgent__")
		}
		if tool != "" {
			status, _ := typed["status"].(string)
			category := classifyToolResult(typed["error"], typed["result"])
			argumentKeys := ""
			if arguments, ok := typed["arguments"].(map[string]any); ok {
				keys := make([]string, 0, len(arguments))
				for key := range arguments {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				argumentKeys = ":args=" + strings.Join(keys, "+") + fmt.Sprintf(":fixture_match=%t", syntheticArgumentsMatch(tool, arguments))
			}
			messageTokens := ""
			if failure, ok := typed["error"].(map[string]any); ok {
				if message, ok := failure["message"].(string); ok {
					messageTokens = ":tokens=" + allowedDiagnosticTokens(message)
				}
			}
			if status != "" || category != "none" || argumentKeys != "" {
				*diagnostics = append(*diagnostics, tool+":"+status+":"+category+argumentKeys+messageTokens)
			}
		}
		for _, item := range typed {
			collectDiagnostics(item, diagnostics)
		}
	}
}

func syntheticArgumentsMatch(tool string, arguments map[string]any) bool {
	if arguments["workspace_id"] != "wsp_fixture" {
		return false
	}
	suffix := "codex"
	switch tool {
	case "begin_work":
		return arguments["idempotency_key"] == "begin_"+suffix+"_1" && arguments["title"] == "Bounded lifecycle" && arguments["outcome"] == "Prove the complete lifecycle."
	case "update_intent":
		revision, ok := arguments["revision"].(float64)
		return ok && revision == 1 && arguments["idempotency_key"] == "update_"+suffix+"_1"
	case "check_coordination":
		budget, ok := arguments["approximate_token_budget"].(float64)
		return ok && budget == 400 && arguments["trigger"] == "before_broad_edit"
	case "report_checkpoint":
		return arguments["checkpoint_id"] == "chk_"+suffix+"_1" && arguments["summary"] == "Bounded checkpoint passed"
	case "acknowledge_context":
		return arguments["idempotency_key"] == "ack_"+suffix+"_1" && arguments["brief_id"] == "brf_fixture"
	case "report_event":
		return arguments["idempotency_key"] == "event_"+suffix+"_1" && arguments["kind"] == "decision"
	case "finish_work":
		return arguments["idempotency_key"] == "finish_"+suffix+"_1" && arguments["outcome"] == "Lifecycle delivered"
	default:
		return false
	}
}

func allowedDiagnosticTokens(message string) string {
	allowed := map[string]bool{
		"additional": true, "argument": true, "available": true, "call": true, "client": true,
		"closed": true, "connect": true, "content": true, "decode": true, "error": true,
		"expected": true, "failed": true, "initialize": true, "invalid": true, "local": true,
		"message": true, "missing": true, "mcp": true, "null": true, "object": true,
		"output": true, "permission": true, "property": true, "protocol": true, "received": true,
		"request": true, "required": true, "result": true, "sandbox": true, "schema": true,
		"server": true, "service": true, "session": true, "overgent": true, "structured": true,
		"tool": true, "tools": true, "unavailable": true, "unknown": true, "validate": true,
		"validation": true, "workspace": true,
	}
	seen := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(message), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if allowed[token] {
			seen[token] = struct{}{}
		}
	}
	tokens := make([]string, 0, len(seen))
	for token := range seen {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	if len(tokens) == 0 {
		return "unclassified"
	}
	return strings.Join(tokens, "+")
}

func classifyToolResult(values ...any) string {
	encoded, _ := json.Marshal(values)
	value := strings.ToLower(string(encoded))
	for category, patterns := range map[string][]string{
		"sandbox_denied":      {"operation not permitted", "permission denied", "sandbox"},
		"socket_unreachable":  {"connect service", "no such file", "connection refused", "invalid argument", "broken pipe", "dial unix"},
		"input_decode":        {"decode", "unmarshal", "unknown field", "missing field"},
		"contract_validation": {"failed to validate", "does not match", "structured content", "output validation", "input validation"},
		"mcp_internal":        {"mcp error", "internal error", "tool execution"},
	} {
		for _, pattern := range patterns {
			if strings.Contains(value, pattern) {
				return category
			}
		}
	}
	for _, category := range []string{"workspace_not_registered", "workspace_registration_ambiguous", "workspace not found", "service unavailable", "revision conflict", "idempotency", "invalid", "required", "schema"} {
		if strings.Contains(value, category) {
			return strings.ReplaceAll(category, " ", "_")
		}
	}
	if value != "[null,null]" && value != "[null]" && value != "null" {
		return "present_" + resultShape(values)
	}
	return "none"
}

func resultShape(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case nil:
			parts = append(parts, "nil")
		case string:
			parts = append(parts, "string")
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts = append(parts, "map-"+strings.Join(keys, "+"))
		default:
			parts = append(parts, fmt.Sprintf("%T", value))
		}
	}
	return strings.Join(parts, "_")
}

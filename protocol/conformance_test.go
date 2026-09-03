package protocol_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	protocoltypes "github.com/overgent/overgent/protocol/generated/go"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestFixtureConformsToSchemaAndGeneratedType(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	files, err := filepath.Glob("schemas/*.json")
	if err != nil {
		t.Fatal(err)
	}
	var schemaURLs []string
	for _, file := range files {
		schemaData, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var schemaDocument map[string]any
		if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
			t.Fatal(err)
		}
		schemaURL, ok := schemaDocument["$id"].(string)
		if !ok {
			t.Fatalf("%s has no $id", file)
		}
		if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
			t.Fatal(err)
		}
		schemaURLs = append(schemaURLs, schemaURL)
	}
	for _, schemaURL := range schemaURLs {
		if _, err := compiler.Compile(schemaURL); err != nil {
			t.Fatalf("compile %s: %v", schemaURL, err)
		}
	}
	const schemaURL = "https://schemas.overgent.com/v1/event-envelope.schema.json"
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}

	fixture, err := os.ReadFile("fixtures/workspace-manifest-completed.json")
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(fixture, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("schema validation: %v", err)
	}
	invalid := value.(map[string]any)
	invalid["payload"].(map[string]any)["pathCount"] = float64(1)
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("type-specific payload accepted an undeclared field")
	}
	delete(invalid["payload"].(map[string]any), "pathCount")

	batchJSON := append([]byte(`{"events":[`), fixture...)
	batchJSON = append(batchJSON, []byte(`]}`)...)
	var batch protocoltypes.PublishEventBatchJSONBody
	if err := json.Unmarshal(batchJSON, &batch); err != nil {
		t.Fatalf("generated Go type: %v", err)
	}
	if len(batch.Events) != 1 || !batch.Events[0].Source.Valid() || !batch.Events[0].Type.Valid() {
		t.Fatalf("generated Go type lost required enum semantics: %#v", batch.Events)
	}
	for _, name := range []string{
		"fixtures/agent-activity-reported.json",
		"fixtures/contract-fingerprints-reported.json",
		"fixtures/read-set-reported.json",
	} {
		eventFixture, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var eventValue any
		if err := json.Unmarshal(eventFixture, &eventValue); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(eventValue); err != nil {
			t.Fatalf("%s schema validation: %v", name, err)
		}
		payload := eventValue.(map[string]any)["payload"].(map[string]any)
		payload["undeclared"] = "x"
		if err := schema.Validate(eventValue); err == nil {
			t.Fatalf("%s accepted an undeclared payload field", name)
		}
	}
}

// The stale-assumption contract extension must be an optional addition to the
// existing evidence shape, not a second evidence contract.
func TestContractEvidenceExtendsTheFindingContract(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	schemaData, err := os.ReadFile("schemas/finding.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument map[string]any
	if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	const schemaURL = "https://schemas.overgent.com/v1/finding.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	finding := map[string]any{
		"id": "fnd_contract_fixture", "kind": "stale_assumption", "severity": "high",
		"confidenceBand": "deterministic", "workstreamIds": []any{"wrk_reader"},
		"reason": "Rotate changed after this session read it.", "state": "open", "revision": float64(1),
		"evidence": []any{map[string]any{
			"kind": "symbol", "summary": "Rotate changed signature.", "source": "git", "fidelity": "structural",
			"contract": map[string]any{
				"path": "internal/session/rotate.go",
				"changedSymbols": []any{map[string]any{
					"name": "Rotate", "oldSignature": "func Rotate(key string) error",
					"newSignature": "func Rotate(ctx context.Context, key string) error",
				}},
				"changedByWorkstreamId": "wrk_writer",
				"readAt":                "2026-08-26T08:59:00Z",
				"changedAt":             "2026-08-26T09:00:00Z",
			},
		}},
	}
	if err := schema.Validate(finding); err != nil {
		t.Fatalf("contract evidence rejected: %v", err)
	}
	evidence := finding["evidence"].([]any)[0].(map[string]any)
	delete(evidence, "contract")
	if err := schema.Validate(finding); err != nil {
		t.Fatalf("contract evidence must stay optional: %v", err)
	}
	evidence["contract"] = map[string]any{"path": "internal/session/rotate.go"}
	if err := schema.Validate(finding); err == nil {
		t.Fatal("incomplete contract evidence was accepted")
	}
}

func TestManifestFixtureRetainsSimultaneousGitStates(t *testing.T) {
	schemaData, err := os.ReadFile("schemas/change-manifest.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument map[string]any
	if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://schemas.overgent.com/v1/change-manifest.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("fixtures/change-manifest-simultaneous.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(fixture, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("schema validation: %v", err)
	}
	entries := value["entries"].([]any)
	states := entries[0].(map[string]any)["states"].(map[string]any)
	if len(states) != 3 {
		t.Fatalf("simultaneous states collapsed: %#v", states)
	}
}

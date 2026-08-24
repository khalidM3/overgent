package protocol_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	protocoltypes "github.com/stickguy/stickguy/protocol/generated/go"
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
	const schemaURL = "https://schemas.stickguy.dev/v1/event-envelope.schema.json"
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

	batchJSON := append([]byte(`{"events":[`), fixture...)
	batchJSON = append(batchJSON, []byte(`]}`)...)
	var batch protocoltypes.PublishEventBatchJSONBody
	if err := json.Unmarshal(batchJSON, &batch); err != nil {
		t.Fatalf("generated Go type: %v", err)
	}
	if len(batch.Events) != 1 || !batch.Events[0].Source.Valid() || !batch.Events[0].Type.Valid() {
		t.Fatalf("generated Go type lost required enum semantics: %#v", batch.Events)
	}
}

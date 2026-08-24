// Command fake-producer emits one synthetic event for contract and demo fixtures.
package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

func main() {
	event := map[string]any{
		"schemaVersion": 1,
		"eventId":       "evt_fake_producer",
		"projectId":     "prj_fixture",
		"memberId":      "mem_fixture",
		"deviceId":      "dev_fixture",
		"workspaceId":   "wsp_fixture",
		"sessionId":     "ses_fixture",
		"sequence":      1,
		"observedAt":    time.Date(2026, 8, 23, 18, 30, 0, 0, time.UTC),
		"sentAt":        time.Date(2026, 8, 23, 18, 30, 1, 0, time.UTC),
		"source":        "manual",
		"type":          "activity.reported",
		"payload": map[string]any{
			"kind": "completion", "summary": "Synthetic producer fixture completed.",
		},
	}
	if err := json.NewEncoder(os.Stdout).Encode(event); err != nil {
		log.Fatalf("encode synthetic event: %v", err)
	}
}

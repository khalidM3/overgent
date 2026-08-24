package main

import "testing"

func TestBoundedContext(t *testing.T) {
	context := "Stickguy fixture brief brf_fixture_1 at context revision 7. Fidelity: fixture_structural. Before broad/shared edits call check_coordination. No source, prompt, transcript, environment, command, patch, or tool output was read."
	if len(context) > 512 {
		t.Fatalf("context too large: %d", len(context))
	}
}

package main

import "testing"

func TestEvaluationSeed(t *testing.T) {
	rep, err := evaluateFile("corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	if rep.CandidateRecall != 1 {
		t.Fatalf("candidate recall = %v, want 1", rep.CandidateRecall)
	}
	if rep.FalsePositives == 0 {
		t.Fatal("seed must retain at least one false positive for precision evaluation")
	}
	for _, r := range rep.Results {
		if (r.CaseID == "repository_isolation" || r.CaseID == "completed_is_ineligible") && (r.Structural || r.LexicalCandidate || r.EmbeddingCandidate) {
			t.Fatalf("ineligible case %s entered candidate set", r.CaseID)
		}
		if r.CaseID == "unrelated_fourth" && len(r.RouteTo) != 0 {
			t.Fatal("unrelated fourth workstream received routed context")
		}
	}
}

func TestStructuralOverlap(t *testing.T) {
	if !overlaps([]string{"a/b.go"}, []string{"a/b.go"}) {
		t.Fatal("same path should overlap")
	}
	if overlaps([]string{"a/b.go"}, []string{"a/c.go"}) {
		t.Fatal("different paths should not overlap")
	}
}

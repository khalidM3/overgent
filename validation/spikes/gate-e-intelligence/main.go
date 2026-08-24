package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"
)

type corpus struct {
	CorpusVersion    int          `json:"corpusVersion"`
	EmbeddingFixture string       `json:"embeddingFixture"`
	Workstreams      []workstream `json:"workstreams"`
	Cases            []evalCase   `json:"cases"`
}

type workstream struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repositoryId"`
	Status       string    `json:"status"`
	Summary      string    `json:"summary"`
	Paths        []string  `json:"paths"`
	Dependencies []string  `json:"dependencies,omitempty"`
	PathCount    int       `json:"pathCount,omitempty"`
	Vector       []float64 `json:"vector"`
}

type evalCase struct {
	ID                string   `json:"id"`
	Left              string   `json:"left"`
	Right             string   `json:"right"`
	ExpectedCandidate bool     `json:"expectedCandidate"`
	ExpectedKind      string   `json:"expectedKind"`
	RouteTo           []string `json:"routeTo"`
}

type result struct {
	CaseID             string   `json:"caseId"`
	Eligible           bool     `json:"eligible"`
	Structural         bool     `json:"structuralCandidate"`
	LexicalScore       float64  `json:"lexicalScore"`
	LexicalCandidate   bool     `json:"lexicalCandidate"`
	EmbeddingScore     float64  `json:"embeddingScore"`
	EmbeddingCandidate bool     `json:"embeddingCandidate"`
	ExpectedCandidate  bool     `json:"expectedCandidate"`
	CandidateRecall    bool     `json:"candidateRecall"`
	FalsePositive      bool     `json:"falsePositive"`
	ExpectedKind       string   `json:"expectedKind,omitempty"`
	RouteTo            []string `json:"routeTo"`
}

type report struct {
	CorpusVersion      int      `json:"corpusVersion"`
	EmbeddingFixture   string   `json:"embeddingFixture"`
	LexicalThreshold   float64  `json:"lexicalThreshold"`
	EmbeddingThreshold float64  `json:"embeddingThreshold"`
	Results            []result `json:"results"`
	CandidateRecall    float64  `json:"candidateRecall"`
	FalsePositives     int      `json:"falsePositives"`
}

const (
	lexicalThreshold   = 0.20
	embeddingThreshold = 0.75
)

func main() {
	path := "corpus.json"
	if len(os.Args) == 2 {
		path = os.Args[1]
	}
	rep, err := evaluateFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func evaluateFile(path string) (report, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return report{}, fmt.Errorf("read corpus: %w", err)
	}
	var c corpus
	if err := json.Unmarshal(b, &c); err != nil {
		return report{}, fmt.Errorf("decode corpus: %w", err)
	}
	byID := make(map[string]workstream, len(c.Workstreams))
	for _, w := range c.Workstreams {
		if w.ID == "" || w.RepositoryID == "" || len(w.Vector) == 0 {
			return report{}, fmt.Errorf("invalid workstream %q", w.ID)
		}
		byID[w.ID] = w
	}
	rep := report{CorpusVersion: c.CorpusVersion, EmbeddingFixture: c.EmbeddingFixture, LexicalThreshold: lexicalThreshold, EmbeddingThreshold: embeddingThreshold}
	positives, recalled := 0, 0
	for _, tc := range c.Cases {
		left, lok := byID[tc.Left]
		right, rok := byID[tc.Right]
		if !lok || !rok {
			return report{}, fmt.Errorf("case %s references unknown workstream", tc.ID)
		}
		eligible := left.Status == "active" && right.Status == "active" && left.RepositoryID == right.RepositoryID
		structural := eligible && (overlaps(left.Paths, right.Paths) || overlaps(left.Dependencies, right.Dependencies))
		lexical := jaccard(tokens(left.Summary), tokens(right.Summary))
		embedding := cosine(left.Vector, right.Vector)
		lexicalCandidate := eligible && lexical >= lexicalThreshold
		embeddingCandidate := eligible && embedding >= embeddingThreshold
		anyCandidate := structural || lexicalCandidate || embeddingCandidate
		if tc.ExpectedCandidate {
			positives++
			if anyCandidate {
				recalled++
			}
		}
		fp := anyCandidate && !tc.ExpectedCandidate
		if fp {
			rep.FalsePositives++
		}
		rep.Results = append(rep.Results, result{
			CaseID: tc.ID, Eligible: eligible, Structural: structural,
			LexicalScore: round(lexical), LexicalCandidate: lexicalCandidate,
			EmbeddingScore: round(embedding), EmbeddingCandidate: embeddingCandidate,
			ExpectedCandidate: tc.ExpectedCandidate, CandidateRecall: tc.ExpectedCandidate && anyCandidate,
			FalsePositive: fp, ExpectedKind: tc.ExpectedKind, RouteTo: append([]string(nil), tc.RouteTo...),
		})
	}
	if positives > 0 {
		rep.CandidateRecall = round(float64(recalled) / float64(positives))
	}
	return rep, nil
}

func overlaps(a, b []string) bool {
	seen := make(map[string]struct{}, len(a))
	for _, p := range a {
		seen[p] = struct{}{}
	}
	for _, p := range b {
		if _, ok := seen[p]; ok {
			return true
		}
	}
	return false
}

func tokens(s string) []string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	set := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if len(part) > 2 {
			set[part] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for part := range set {
		out = append(out, part)
	}
	sort.Strings(out)
	return out
}

func jaccard(a, b []string) float64 {
	left := make(map[string]struct{}, len(a))
	for _, s := range a {
		left[s] = struct{}{}
	}
	intersection := 0
	union := len(left)
	for _, s := range b {
		if _, ok := left[s]; ok {
			intersection++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }

# Gate E evidence

Outcome: **pass** for the bounded evaluation-seed gate.

## Environment

- Go: 1.26.7 darwin/arm64
- Corpus: `corpus.json`, version 1
- Embedding fixture: `synthetic-concept-vector-v1`
- External/project data: none; all records are synthetic

## Reproduction

From this directory, with isolated caches:

```bash
GOCACHE=/private/tmp/stickguy-l1/go-cache \
GOMODCACHE=/private/tmp/stickguy-l1/go-mod-cache \
go test ./...

GOCACHE=/private/tmp/stickguy-l1/go-cache \
GOMODCACHE=/private/tmp/stickguy-l1/go-mod-cache \
go run . corpus.json
```

Observed on 2026-08-23: tests passed; expected-candidate recall was `1.0`;
one false positive was retained.

## Results

| Case | Structural | Lexical | Synthetic vector | Expected candidate | Result |
|---|---:|---:|---:|---:|---|
| duplicate behavior/different paths | no | yes | yes | yes | recalled |
| incompatible intent before edits | no | no | yes | yes | recalled |
| shared schema impact | yes | no | no | yes | recalled |
| independent 1,000-path mechanical changes | no | no | yes | no | false positive |
| related documentation/non-conflicting | no | no | no | no | quiet |
| completed workstream | ineligible | ineligible | ineligible | no | quiet |
| unrelated fourth workstream | no | no | no | no | no route |
| similar text in another repository | ineligible | ineligible | ineligible | no | isolated |

## Interpretation

The seed proves that candidate recall, false-positive accounting, lifecycle and
repository eligibility, expected finding labels, and routing exclusions are
executable and versioned. The large-mechanical-change false positive is useful
negative evidence: vector proximity alone cannot produce a proactive finding.

The synthetic concept vectors do not benchmark or select a production embedding
provider. Provider evaluation remains an L6 responsibility, and proactive
semantic notifications stay disabled until the owner-approved precision gate.

## Privacy/security

No source, diff, prompt, transcript, environment value, credential, or customer
record is present. Similar text in `repo_isolation` scores `1.0` but cannot enter
the candidate set because eligibility applies repository scope first.

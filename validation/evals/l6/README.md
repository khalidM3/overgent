# L6 public intelligence evaluation

The executable corpus lives in
`packages/coordination/test/intelligence.test.ts`. It uses synthetic Project,
repository, workstream, path, contract, and summary values only.

Run the labeled evaluation with:

```bash
pnpm --dir packages/coordination test -- intelligence.test.ts
```

The corpus covers duplicate behavior under different paths, incompatible
session assumptions before path overlap, shared dependency impact, every public
finding kind, independent mechanical changes, related documentation that must
stay quiet, completed and repository-isolated work, an unrelated workstream
that receives no brief item, budget/truncation, relevant-only staleness,
provider outage, exact semantic-index scope filtering, and adversarial semantic
text. Expected labels are assertions; a changed classification fails the gate.

The deterministic concept-vector adapter is a privacy-preserving initial
provider, not a claim of general language understanding. Its semantic and
lexical signals may create quiet radar findings, but Overgent has no proactive
semantic interruption channel at L6. Broader precision measurement must precede
one.

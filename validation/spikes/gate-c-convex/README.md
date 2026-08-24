# Gate C — Convex feasibility spike

This directory is a disposable Convex 1.45.0 project using synthetic records.
It is not production backend code.

The spike exercises transactional event deduplication, atomic manifest activation,
repository-scope revisions, mandatory vector scope filtering, post-retrieval
authorization/current-state reload, update/deletion/model migration, bounded
race fallback, realtime subscription, retention deletion, and a 1,000-path
manifest split into bounded chunks.

Local deployment state, generated code, URLs, and credentials are ignored and
must not be committed.

Run instructions, redacted evidence, and the proposed decision are in
`evidence/2026-08-23-live-run.md` and `OUTCOME.md`.

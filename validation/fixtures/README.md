# Shared synthetic fixture vocabulary

`l1-scope-v1.json` fixes the identifiers and minimum vocabulary shared by Gates
C and E before those gates run. It is validation input, not an external API
schema.

The fixture deliberately includes:

- semantically overlapping authentication work under different paths;
- a shared membership-schema dependency;
- an unrelated fourth workstream that must receive no context; and
- similar text in a different repository that must never enter retrieval.

Gate-specific fixtures may extend these records locally, but must preserve the
IDs, repository boundaries, path-only manifest rule, and expected routing labels.

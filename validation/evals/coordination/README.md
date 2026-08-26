# Coordination evaluation harness

Run the permanent M1 gate from the repository root:

```bash
pnpm eval:coordination
```

The command starts the documented anonymous loopback backend, materializes the
committed fixture as a temporary Git repository with two linked worktrees, runs
all seven scripted two-agent scenarios through the real local service and stdio
MCP bridge, prints a human-readable result table, and writes
`coordination-eval-report.json` in the repository root.

Assertions are tagged `structural`, `contract`, `injection`, `semantic`, or
`dependency`. `capabilities.json` is the integrator-owned switch that makes a
capability required. Assertions for capabilities not listed there are reported
as `not_yet_implemented`; they are never omitted. The command exits nonzero on
an execution error or a failed assertion whose capability is required.

The report contains only synthetic fixture identifiers, paths, finding
evidence, and aggregate timing/routing metrics. Temporary repositories, local
service state, credentials, and backend logs are removed at the end of a run.

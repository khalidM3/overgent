# Validation

Executable evaluation harnesses for Overgent's coordination behavior. Nothing
here is production application code, and every fixture is synthetic.

| Path | What it is |
| --- | --- |
| `evals/coordination/` | The coordination gate: seven scripted two-agent scenarios run through the real local service and stdio MCP bridge. `pnpm eval:coordination` |
| `evals/l6/` | The public intelligence evaluation, whose executable corpus lives in `packages/coordination/test/intelligence.test.ts`. |

## Privacy boundary

All fixtures are synthetic. Results committed here must contain no credentials,
account identifiers, transcript paths, prompt text, source or diff content,
environment values, or raw command output beyond what establishes an outcome.

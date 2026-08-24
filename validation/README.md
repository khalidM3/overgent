# L-1 validation workspace

This directory contains bounded, disposable architecture spikes and the evidence
needed by `docs/prebuild-validation.md`. Nothing here is production application
code.

## Integration ownership

The root integrator owns `validation/fixtures/`, accepted ADR outcomes, and the
final L-1 gate matrix. Gate lanes may add files only beneath their own
`validation/spikes/gate-*` directory and must not redefine shared fixture fields
or production contracts.

## Privacy boundary

All fixtures are synthetic. Evidence must redact credentials, account IDs,
transcript paths, prompt text, source/diff content, environment values, and raw
command/test output that is not necessary to establish a gate outcome.

## Gate outcomes

Each gate ends in exactly one documented outcome: `pass`, `narrow`, `replace`,
or `block`. A pass proves only the bounded assumption named by the gate.

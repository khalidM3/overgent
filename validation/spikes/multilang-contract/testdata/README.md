# Spike fixtures

Real source files, not synthetic ones: the spike's whole point is whether a
grammar handles code people actually ship. Third-party files are copied
verbatim and are inputs to a disposable validation spike, not distributed
components of Stickguy.

| File | Origin | Licence |
| --- | --- | --- |
| `dataclasses.py`, `argparse.py` | CPython 3.13.2 standard library | PSF License Agreement |
| `uri.js` | `uri-js` npm package, `dist/esnext/uri.js` (ESM) | BSD-2-Clause |
| `convertPathData.js`, `_path.js` | `svgo` npm package, `plugins/` (CommonJS) | MIT |
| `typescript.go.txt` | this repository, `internal/contract/typescript.go` | Apache-2.0 |
| `domain.ts.txt` | this repository, `convex/src/domain.ts` | Apache-2.0 |
| `app.test.tsx.txt` | this repository, `apps/dashboard/test/app.test.tsx` | Apache-2.0 |
| `typescript-sample.ts.txt` | this repository, `packages/coordination/src/intelligence.ts` | Apache-2.0 |

This repository's own files carry a `.txt` suffix so the root module's build,
vet, and test commands do not try to compile them.

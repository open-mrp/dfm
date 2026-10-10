# dfm

A small, deterministic design-for-manufacturing (DFM) engine written in Go. It answers one question: can this design be manufactured with this capability and this stock?

```go
Check(design, capability, stock) -> result
```

## Scope


| Supported today                                                                                                  | Not supported (yet)                                                                                                           |
| ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| DXF (ASCII and binary) and SVG designs; STEP models of sheet-metal parts, unfolded                               | Other 3D geometry, DWG, PDF/AI/EPS                                                                                            |
| Laser cutting and press-brake bending, planned as operations with countersinking, tapping and hardware insertion | Other processes (machining, tube, welding, finishing)                                                                         |
| Capability + stock as JSON; Go library and CLI                                                                   |                                                                                                                               |
| Taps, countersinks and press-in fasteners on holes                                                               | Blind holes, counterbores, formed taps, hems, tooling collisions when the capability gives no tooling (reviewed, not checked) |


The design is process-generic: a capability names its `process` and carries a block of the same name (`laser_cutting`), materials carry per-process blocks, and each process has its own rule set (`rules.ForProcess`). Adding CNC means a new process constant, a `cnc_milling` block and rules; the geometry-hygiene rules are shared.

## Quick start

```bash
go install github.com/open-mrp/dfm/cmd/dfm@latest

dfm check fixtures/valid/valid_rectangle.dxf --capability examples/laser.json \
  --material mild_steel --thickness 1.5mm
# PASS fixtures/valid/valid_rectangle.dxf: manufacturable (0 errors, 0 warnings)
```

Add `--json` for a `dfm_check` resource. `dfm plan` checks every operation a design needs (cutting, hole operations, bending), `dfm fix` writes a corrected file, `dfm preview` draws the design as read with its findings marked, and `dfm codes` lists every finding code. Exit codes: 0 manufacturable, 1 not manufacturable, 2 invalid input or error, 3 needs review.

From Go, call `dfm.Check(ctx, dfm.CheckRequest{Design: ..., Capability: ..., Stock: ...})`.

## How it works

```text
request JSON ──request.Decode──> capability.Capability + capability.Stock ──Resolve──> Profile
                                                                                          │
design file ──design.Parse──> geometry.Part ──rules.ForProcess(process)──> []rules.Finding
                                                                                          │
                                        dfm.Check (service) ──resource.PresentCheck──> response JSON
```

- Results are grouped by part, and each finding carries its measured value, the requirement, a location and, where the engine can tell what was meant, a fix to accept.
- Lengths are exact integers in nanometers and decimals travel as strings, so no verdict depends on floating-point error or the CPU.
- A check has no side effects and no global state: the same input always gives the same result.



## Documentation

- [docs/checks.md](docs/checks.md): every check, its finding code, severity and when it fails.
- [docs/designs.md](docs/designs.md): how DXF, SVG and STEP files are read, and their limits.
- [docs/capabilities.md](docs/capabilities.md): the reference machine presets.
- [docs/development.md](docs/development.md): architecture, fixtures, adding a rule and running tests.
- [docs/private-corpus.md](docs/private-corpus.md): the real drawings the engine is measured against.



## Development

```bash
go test ./...                       # everything (standard library only)
go run ./fixtures/gen               # regenerate generated fixtures
go test . -run TestFixtures -update # rewrite golden results, then review the diff
go run ./schema/gen                 # regenerate JSON Schemas
```

See [AGENTS.md](AGENTS.md) for the rules a change must follow.

## License

The engine is licensed under the Apache License, Version 2.0 ([LICENSE](LICENSE)). The real drawings it is tested against are not part of it: each keeps its project's license beside it in the private corpus, recorded with its source in the corpus manifest. Every file under `fixtures/` was made for this repository.
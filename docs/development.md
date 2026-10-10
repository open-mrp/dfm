# Development

## Packages

| Package          | Role                                                                         |
| ---------------- | ---------------------------------------------------------------------------- |
| `dfm` (root)     | `Check`, `CheckPlan`, `Repair`, `Preview`: the service every transport calls |
| `request`        | Strict wire decoding; errors carry a `param` path                            |
| `resource`       | Response resources and the presenter                                         |
| `apierror`       | The error type every layer returns                                           |
| `constants`      | Enums: processes, formats, units, finding and error codes                    |
| `field`          | `Optional[T]` request fields                                                 |
| `measure`        | Exact lengths (integer nm), ratios and decimals                              |
| `capability`     | The capability model, validation and `Resolve`                               |
| `design`         | DXF, SVG and STEP → geometry (the only format-aware package)                 |
| `solid`, `sheet` | The STEP boundary model and its unfolding                                    |
| `geometry`       | Contours, circles, slots, containment, exact distances                       |
| `rules`          | The checks, one file per topic, and the catalog of codes                     |
| `brake`          | The press-brake simulation: bend order and collisions                        |
| `capabilities`   | The reference presets                                                        |
| `preview`        | The design drawn as SVG with findings marked                                 |
| `feedback`       | Replaying stored checks and building feedback fixtures                       |
| `cmd/dfm`        | The CLI                                                                      |
| `schema`         | JSON Schemas generated from `request` and `resource`                         |

## Fixtures

`fixtures/` is part of the product. Each design has a sibling `*.expected.json` with its exact response for `examples/laser.json` and 1.5 mm `mild_steel`, unless a `<name>.stock.json`, `<name>.capability.json` or `<name>.hole_operations.json` sidecar says otherwise. `TestFixtures` checks every file against its golden response and the schemas, and fails if any rule is not exercised.

| Directory             | Expectation                                           |
| --------------------- | ----------------------------------------------------- |
| `fixtures/valid/`     | `manufacturable`, or warnings only                    |
| `fixtures/invalid/`   | `not_manufacturable`                                  |
| `fixtures/plan/`      | Plans, each with a `<name>.plan.json` sidecar         |
| `fixtures/malformed/` | An error (exit 2)                                     |
| `fixtures/exported/`  | Real exports from CAD software, reviewed by hand      |
| `fixtures/feedback/`  | Regression tests from user reports                    |
| `fixtures/perf/`      | Generated drawings for benchmarks and `TestCheckTime` |

Generated fixtures come from `fixtures/gen` as complete DXF R2000 drawings; `go test ./fixtures/gen` fails if the committed files differ from the generator's output.

Real drawings and outside datasets live in the private corpus ([private-corpus.md](private-corpus.md)). `TestPrivateCorpus` checks them and logs a scorecard with `-v`; `TestExternalSuites` and `TestExternalBends` run the outside data when it has been fetched (`DFM_BENDS_FULL=1` for the whole bend dataset).

## Adding a rule

1. Add a `constants.FindingCode`.
2. Add any new limit to `capability/model.go` (validation and `Resolve`) and `request/types.go`, then `go run ./schema/gen`. A new required field bumps `schema_version`.
3. Compute any new derived geometry in `geometry`; rules never see DXF.
4. Write `rules/<name>.go` with `Code()` and `Check(part, profile)`, and add it to `rules.ForProcess`. Convert with `measure.FromMM` and compare with `checkLimit`; never compare floats against limits.
5. Add table-driven tests in `rules/` and a fixture in `fixtures/gen`, then regenerate the goldens.
6. Add a row to [checks.md](checks.md).

## Running tests

```bash
go test ./...                                  # everything (standard library only)
go test -race ./...
go run ./fixtures/gen                          # regenerate fixtures
go test . -run TestFixtures -update            # rewrite golden results, then review the diff
go test . -run '^$' -fuzz FuzzCheck            # fuzz the pipeline (also FuzzParseDXF, FuzzParseSTEP in ./design, FuzzFlatten in ./sheet)
go test -run '^$' -bench . -benchmem -count 6 . ./brake > /tmp/bench.txt
go run golang.org/x/perf/cmd/benchstat@latest testdata/bench/baseline.txt /tmp/bench.txt
```

`TestCheckTime` fails a check that blows its time budget. CI benchmarks the base and head of a change on the same runner and fails a case 30% slower; re-record `testdata/bench/baseline.txt` in the same commit when a change is meant to move it.

# AGENTS.md

A deterministic design-for-manufacturing engine in Go (`github.com/open-mrp/dfm`): `Check(design, capability, stock) -> result`. This repo is the engine (checks, fixes, corrected files, and what they need); the platform API and the MCP server live in a separate repo. Read `README.md` for scope and the API and `docs/checks.md` for every check.

## Commands

Run everything from the repository root.

```sh
go test ./...                 # all tests
go vet ./...
go run ./schema/gen           # regenerate JSON Schemas after changing request/ or resource/ types
go run ./fixtures/gen         # regenerate generated fixtures
go run ./cmd/dfm --help       # CLI: check, plan, fix, preview, replay, feedback-fixture, codes, capabilities
```

## Layout

- `design` is the only format-aware package (DXF, SVG, STEP → `geometry.Part`).
- `rules` holds the checks, one file per topic (hygiene, part size, spacing, bends, ...); rule sets are chosen per process with `rules.ForProcess`, and `catalog.go` says what each code means, when it is skipped and whether it is about the drawing or a part.
- `request` decodes wire JSON strictly; `resource` presents responses; errors are `apierror`.
- Finding codes, error codes and other enums live in `constants` and are stable identifiers.
- Lengths are exact integers in nm (`measure`); decimals go over the wire as strings.
- No global state: configuration is passed in.

## Rules

- Keep results deterministic. Do not rename or remove finding or error codes.
- Results must not depend on the CPU. Go fuses `a*b + c` into one instruction on arm64 but not on amd64, which can move a coordinate by one bit and flip a finding. Write `float64(a*b) + c` in coordinate arithmetic, and check both with `GOARCH=amd64 go test ./...` (Rosetta on Apple silicon). `go run golang.org/x/tools/cmd/bisect -compile=fma go test ...` finds a fused line that changes a result.
- Golden files hold `"engine_version": "dev"`. When results change, run `go test . -update` and title the PR `feat:` or `fix:`. Release-please sets `EngineVersion`; never edit it by hand.
- PR titles are conventional commits (`fix(rules): ...`); PRs are squash-merged and the title becomes the commit and the changelog entry.
- A new or changed check needs a fixture with a known outcome and a row in `docs/checks.md`.
- Never commit compiled binaries (`scripts/check-no-binaries`).
- Never commit, print or upload anything from `fixtures/private/` (see `docs/private-corpus.md`).
- Every real drawing is in the private corpus (`fixtures/private/`, fetched by maintainers with `scripts/private-corpus fetch`): published open-hardware parts and proprietary ones alike, under opaque case IDs. Nothing in the repository may name a project in it or carry one of its drawings; the synthetic fixtures are the public test data.
- Record the reasoning behind a non-obvious decision in the package comment or the doc comment of the code that makes it; keep `README.md` and `docs/` current in the same commit.
- Match the README's plain style in docs and comments.
- Do not hard-wrap Markdown: write each paragraph or list item on one line and let the editor wrap it.
- Do not hard-wrap prose in Go comments either: one paragraph is one `//` line. Older comments are wrapped; reflow one when you edit it, not otherwise.
- Follow the platform API's shape conventions: every resource has `object`; no `omitempty` on responses (`null`, never absent); enums, not booleans, for states; instants end in `_at`; the error envelope is `type`, `code`, `message`, `param`, `is_transient`; quantities are `{value, unit}` with string decimals; optional inputs are `field.Optional`. Shape changes after 1.0 are additive only.
- Speed is a feature. `TestCheckTime` sets budgets, and `testdata/bench/baseline.txt` records `BenchmarkCheck`, `BenchmarkPlan` and `BenchmarkSequence`. Before committing a change to `geometry`, `rules`, `design`, `sheet` or `brake`, run `go test -run '^$' -bench . -benchmem -count 6 . ./brake > /tmp/bench.txt` and compare with `go run golang.org/x/perf/cmd/benchstat@latest testdata/bench/baseline.txt /tmp/bench.txt`. A slowdown needs a reason in the commit message; re-record the baseline in the same commit when the change is meant to move it. CI benchmarks the base and the head of a change on the same runner. Indexes and caches pay off only above a size; keep a plain scan for small parts and measure both (the typical check is 150 µs).

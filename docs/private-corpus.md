# Private corpus

Every real drawing the engine is measured against (published open-hardware parts and proprietary parts from shops) lives in a private bucket, never in the repository, under opaque case IDs. So do the outside datasets and test suites. The repository's own test data is the generated fixtures, which cover every check.

## Storage

- **Two roles only.** Maintainers read and write with multi-factor authentication. CI reads with short-lived credentials issued only to the corpus workflow on `main`; pull requests never can. The infrastructure role can list file names but not read them.
- **Encrypted** with a dedicated key only those roles can use. No public access or shareable links.
- **Every request is logged** to a separate, undeletable log bucket.
- **Deletion is real**: a deleted file is recoverable for 7 days, then gone. Drawings are kept until their contributor asks for removal.

```text
manifest.json                            # the cases and their sources
rates.json                               # counts per finding code, compared across engine versions
licenses/<project>/LICENSE
<case-id>/<case-id>.<dxf|svg|step|...>
<case-id>/<case-id>.expected.json
capability-*.json
external/manifest.json                   # outside sources: origin, pinned version, license, ID
external/records/<id>.json               # each source's outcomes
external/<id>/...
```

Case IDs (`p0001`, ...) and source IDs (`x01`, ...) never contain a company, customer, product, project or person name: they appear in CI output and commit messages. Nothing in the repository may name a project in the corpus or carry one of its drawings.

## Manifest

| Field                          | Meaning                                                                                                                       |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `file`, `sha256`               | The drawing and its hash; a mismatch fails the case.                                                                          |
| `status`                       | `good`, `bad` or `unknown`.                                                                                                   |
| `evidence`                     | What happened when it was made.                                                                                               |
| `expected`                     | Optional: what the result should be, if not today's golden.                                                                   |
| `stock`, `units`, `capability` | What the case is checked with.                                                                                                |
| `known_errors`                 | Expected errors, each with a reason (`stricter_limit`, `defect`, `assumed_input`), so the scorecard counts them as explained. |
| `contributor`                  | An internal reference, never a name.                                                                                          |
| `consent`                      | Date, scope (`ci_testing`) and expiry.                                                                                        |

## Using it

The bucket and roles are not in the repository. Set `DFM_PRIVATE_CORPUS_BUCKET` and `DFM_PRIVATE_CORPUS_MAINTAINER_ROLE` locally; CI reads `DFM_PRIVATE_CORPUS_BUCKET`, `DFM_PRIVATE_CORPUS_REGION` and `DFM_PRIVATE_CORPUS_READER_ROLE` from the `private-corpus` environment's variables.

```bash
eval "$(scripts/private-corpus session <mfa-code>)"      # one-hour maintainer session
scripts/private-corpus fetch                              # into fixtures/private/
go test . -run TestPrivateCorpus -v                       # scorecard; skipped when not fetched
go test . -run TestPrivateCorpus -update                  # rewrite goldens, then push them
scripts/private-corpus fetch-external [x01 ...]           # outside data (several GB)
go test . -run 'TestExternalBends|TestExternalSuites' -v  # DFM_BENDS_FULL=1 for the whole bend dataset
```

`fixtures/private/` is gitignored and readable by you alone. Do not copy files out of it, attach them anywhere or open them in online tools. Goldens are never committed: upload them with `scripts/private-corpus push <case-id>` (or `push-all`, `push-external`).

## CI

`.github/workflows/private-corpus.yml` runs weekly and on demand (`gh workflow run private-corpus.yml`), on `main` only: it fetches the corpus, runs `TestPrivateCorpus` (which verifies every hash) and deletes it. A weekly `external` job runs the full outside data. Failures report only the case ID and which finding counts changed, never geometry, coordinates or handles. Pull requests do not run the corpus, so maintainers run it locally before merging a change that could move it.

## Adding a drawing

The contributor sends, privately, the file exactly as it was cut, the material and thickness, what happened (made, rejected and why, or failed), and consent for CI testing. Title blocks and names may be removed if the cut geometry is unchanged.

A maintainer checks it holds no personal data, then:

```bash
mkdir fixtures/private/p0007
cp <drawing>.dxf fixtures/private/p0007/p0007.dxf
shasum -a 256 fixtures/private/p0007/p0007.dxf      # for the manifest entry
# add the manifest entry
go test . -run TestPrivateCorpus -update -v         # writes p0007.expected.json
scripts/private-corpus push p0007                   # uploads the case and the manifest
```

If the drawing shows a bug, the fix ships with a minimal generated fixture that reproduces it; the private drawing is never what makes the problem public.

## Removing a drawing

Run `scripts/private-corpus remove <case-id>` (it deletes every version), remove the manifest entry and record the removal date. Nothing in git changes.

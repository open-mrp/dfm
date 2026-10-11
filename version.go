package dfm

// EngineVersion identifies the parser and rule set that produce results, so a reported result can be traced to the engine that produced it and re-checked on a newer one. It is the release version: release-please sets it, and fixtures/results.json, in each release PR, and a change to results is titled feat or fix so the next release moves it (see .github/workflows/pr-title.yml).
const EngineVersion = "0.52.0" // x-release-please-version

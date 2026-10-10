package dfm

// EngineVersion identifies the parser and rule set that produce results. It is bumped whenever any result can change, so a reported result can be traced to the engine that produced it and re-checked on a newer one. TestEngineVersion fails when fixture results change without a bump.
const EngineVersion = "0.51.0"

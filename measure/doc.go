// Package measure provides exact numeric types for DFM limits and results.
//
// I was concerned that floating-point error might cause some flaky results, so here we utilize exact numeric types for all calculations. This provides the added benefit of keeping the results deterministic across CPU architectures.
//
//   - Length is an integer number of nanometers. Lengths are parsed from a decimal string and a unit exactly. A value that is not a whole number of nanometers is rejected rather than rounded.
//   - Area is an integer number of square micrometers, parsed the same way; a square inch or foot is a whole number of them, and a sheet-sized area fits int64 with room to spare.
//   - Ratio is an integer number of millionths, parsed the same way as Length.
//   - Decimal is an exact finite decimal used for computed limits such as 0.333333 × 2.032 mm = 0.677332656 mm, which may be finer than 1 nm.
//   - Measurements taken from geometry are computed in double precision and rounded once to the nearest nanometer (FromMM); all comparisons after that are exact.
package measure

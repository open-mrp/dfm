// Package constants holds the fixed value sets used on the wire and in the domain. Every enum is a string type with named constants, IsValid and EnumValues (declaration order).
package constants

import "slices"

func isOneOf[T ~string](v T, set []T) bool { return slices.Contains(set, v) }

func enumValues[T ~string](set []T) []string {
	out := make([]string, len(set))
	for i, v := range set {
		out[i] = string(v)
	}
	return out
}

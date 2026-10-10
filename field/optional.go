// Package field provides request field types that record presence.
package field

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// Optional is an optional request field. It distinguishes an absent field from a set one; a null reads as absent, and request decoding rejects an explicit null before it gets here. Use it with the ",omitzero" JSON option.
type Optional[T any] struct {
	value T
	set   bool
}

// Some returns a set Optional.
func Some[T any](v T) Optional[T] { return Optional[T]{value: v, set: true} }

// Value returns the value and whether it was set.
func (o Optional[T]) Value() (T, bool) { return o.value, o.set }

// Ptr returns a pointer to the value, or nil when unset.
func (o Optional[T]) Ptr() *T {
	if !o.set {
		return nil
	}
	v := o.value
	return &v
}

// IsZero reports whether the field is unset, for ",omitzero".
func (o Optional[T]) IsZero() bool { return !o.set }

// ElemType returns T, so request decoding can validate the inner value.
func (Optional[T]) ElemType() reflect.Type { return reflect.TypeFor[T]() }

func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.set {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &o.value); err != nil {
		return err
	}
	o.set = true
	return nil
}

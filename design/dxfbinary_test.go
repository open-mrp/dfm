package design

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// toBinary encodes an ASCII DXF as binary DXF, with one-byte group codes (R12) or two-byte ones.
func toBinary(t testing.TB, ascii []byte, oneByte bool) []byte {
	t.Helper()
	tags, err := readTags(ascii)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	b.WriteString(binarySentinel)
	for _, tg := range tags {
		if oneByte && tg.code < 255 {
			b.WriteByte(byte(tg.code))
		} else {
			if oneByte {
				b.WriteByte(255)
			}
			_ = binary.Write(&b, binary.LittleEndian, int16(tg.code))
		}
		switch binaryTypeOf(tg.code) {
		case binDouble:
			v, err := strconv.ParseFloat(tg.value, 64)
			if err != nil {
				t.Fatalf("code %d: %v", tg.code, err)
			}
			_ = binary.Write(&b, binary.LittleEndian, math.Float64bits(v))
		case binInt16, binInt32, binInt64, binBool:
			v, err := strconv.ParseInt(tg.value, 10, 64)
			if err != nil {
				t.Fatalf("code %d: %v", tg.code, err)
			}
			switch binaryTypeOf(tg.code) {
			case binInt16:
				_ = binary.Write(&b, binary.LittleEndian, int16(v))
			case binInt32:
				_ = binary.Write(&b, binary.LittleEndian, int32(v))
			case binInt64:
				_ = binary.Write(&b, binary.LittleEndian, v)
			default:
				b.WriteByte(byte(v))
			}
		case binChunk:
			raw, err := hex.DecodeString(tg.value)
			if err != nil {
				t.Fatalf("code %d: %v", tg.code, err)
			}
			b.WriteByte(byte(len(raw)))
			b.Write(raw)
		default:
			b.WriteString(tg.value)
			b.WriteByte(0)
		}
	}
	return b.Bytes()
}

// TestBinaryMatchesASCII parses every ASCII fixture again as binary DXF, with one- and two-byte group codes, and expects the same part.
func TestBinaryMatchesASCII(t *testing.T) {
	paths, _ := filepath.Glob("../fixtures/*/*.dxf")
	n := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil || bytes.HasPrefix(data, []byte("AutoCAD Binary")) {
			continue
		}
		want, wantErr := ParseDXF(data, "mm")
		if _, err := readTags(data); err != nil {
			continue // a deliberately broken fixture
		}
		for _, oneByte := range []bool{true, false} {
			got, gotErr := ParseDXF(toBinary(t, data, oneByte), "mm")
			if (wantErr == nil) != (gotErr == nil) || wantErr != nil && wantErr.Error() != gotErr.Error() {
				t.Errorf("%s (one-byte codes %v): error %v, want %v", p, oneByte, gotErr, wantErr)
				continue
			}
			if wantErr == nil && !reflect.DeepEqual(got, want) {
				t.Errorf("%s (one-byte codes %v): part differs from the ASCII parse", p, oneByte)
			}
		}
		n++
	}
	if n < 50 {
		t.Fatalf("only %d fixtures compared", n)
	}
}

func TestBinaryTruncated(t *testing.T) {
	full := toBinary(t, []byte(dxf("", circleEnt)), false)
	for _, cut := range []int{len(binarySentinel) + 1, len(full) / 2, len(full) - 3} {
		if _, err := ParseDXF(full[:cut], ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("cut at %d: err = %v, want invalid", cut, err)
		}
	}
}

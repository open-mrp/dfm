package main

import (
	"bytes"
	"encoding/binary"
	encodinghex "encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// binarySentinel starts every binary DXF file.
const binarySentinel = "AutoCAD Binary DXF\r\n\x1a\x00"

// toBinaryDXF re-encodes an ASCII DXF drawing as binary DXF, with one-byte group codes (as R12 writes them) or two-byte ones (R13 and later). Each value is stored in the type its group code's range calls for.
func toBinaryDXF(ascii []byte, oneByte bool) ([]byte, error) {
	lines := strings.Split(strings.TrimSuffix(string(ascii), "\n"), "\n")
	if len(lines)%2 != 0 {
		return nil, fmt.Errorf("odd number of lines")
	}
	var b bytes.Buffer
	b.WriteString(binarySentinel)
	for i := 0; i < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", i+1, err)
		}
		value := strings.TrimSpace(lines[i+1])
		if oneByte && code < 255 {
			b.WriteByte(byte(code))
		} else {
			if oneByte {
				b.WriteByte(255)
			}
			_ = binary.Write(&b, binary.LittleEndian, int16(code))
		}
		switch {
		case code >= 10 && code <= 59, code >= 110 && code <= 149, code >= 210 && code <= 239, code >= 460 && code <= 469, code >= 1010 && code <= 1059:
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			_ = binary.Write(&b, binary.LittleEndian, math.Float64bits(v))
		case code >= 60 && code <= 79, code >= 170 && code <= 179, code >= 270 && code <= 289, code >= 370 && code <= 389, code >= 400 && code <= 409, code >= 1060 && code <= 1070:
			v, err := strconv.ParseInt(value, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			_ = binary.Write(&b, binary.LittleEndian, int16(v))
		case code >= 90 && code <= 99, code >= 420 && code <= 429, code >= 440 && code <= 449, code == 1071:
			v, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			_ = binary.Write(&b, binary.LittleEndian, int32(v))
		case code >= 160 && code <= 169, code >= 450 && code <= 459:
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			_ = binary.Write(&b, binary.LittleEndian, v)
		case code >= 290 && code <= 299:
			v, err := strconv.ParseInt(value, 10, 8)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			b.WriteByte(byte(v))
		case code >= 310 && code <= 319, code == 1004:
			raw, err := encodinghex.DecodeString(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", i+2, err)
			}
			b.WriteByte(byte(len(raw)))
			b.Write(raw)
		default:
			b.WriteString(value)
			b.WriteByte(0)
		}
	}
	return b.Bytes(), nil
}

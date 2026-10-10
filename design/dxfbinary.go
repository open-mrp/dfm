package design

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

// binarySentinel starts every binary DXF file.
const binarySentinel = "AutoCAD Binary DXF\r\n\x1a\x00"

// binaryType is how a group code's value is stored in binary DXF.
type binaryType int

const (
	binString binaryType = iota // NUL-terminated
	binDouble                   // 8-byte IEEE 754
	binInt16
	binInt32
	binInt64
	binBool  // 1 byte
	binChunk // length byte, then that many bytes
)

// binaryTypeOf returns how the value of group code c is stored, following the DXF reference's group code ranges.
func binaryTypeOf(c int) binaryType {
	switch {
	case c >= 10 && c <= 59, c >= 110 && c <= 149, c >= 210 && c <= 239, c >= 460 && c <= 469, c >= 1010 && c <= 1059:
		return binDouble
	case c >= 60 && c <= 79, c >= 170 && c <= 179, c >= 270 && c <= 289, c >= 370 && c <= 389, c >= 400 && c <= 409, c >= 1060 && c <= 1070:
		return binInt16
	case c >= 90 && c <= 99, c >= 420 && c <= 429, c >= 440 && c <= 449, c == 1071:
		return binInt32
	case c >= 160 && c <= 169, c >= 450 && c <= 459:
		return binInt64
	case c >= 290 && c <= 299:
		return binBool
	case c >= 310 && c <= 319, c == 1004:
		return binChunk
	}
	return binString
}

// readBinaryTags decodes a binary DXF file into the tags its ASCII form would have. Each tag's line is the line its group code would be on in that ASCII form, so error messages read the same for both. R12 files store group codes in one byte (255 escaping a two-byte code); later ones in two.
func readBinaryTags(data []byte) ([]tag, error) {
	b := data[len(binarySentinel):]
	if len(b) < 2 {
		return nil, fmt.Errorf("%w: binary DXF is empty", ErrInvalid)
	}
	// The first record is group code 0 "SECTION": a one-byte code is followed directly by 'S'.
	oneByte := b[0] == 0 && b[1] == 'S'
	var tags []tag
	pos := 0
	truncated := func() error {
		return fmt.Errorf("%w: binary DXF is truncated at byte %d", ErrInvalid, len(binarySentinel)+pos)
	}
	for pos < len(b) {
		var code int
		if oneByte {
			code = int(b[pos])
			pos++
			if code == 255 {
				if pos+2 > len(b) {
					return nil, truncated()
				}
				code = int(int16(binary.LittleEndian.Uint16(b[pos:])))
				pos += 2
			}
		} else {
			if pos+2 > len(b) {
				return nil, truncated()
			}
			code = int(int16(binary.LittleEndian.Uint16(b[pos:])))
			pos += 2
		}
		line := 2*len(tags) + 1
		var value string
		switch binaryTypeOf(code) {
		case binDouble:
			if pos+8 > len(b) {
				return nil, truncated()
			}
			value = strconv.FormatFloat(math.Float64frombits(binary.LittleEndian.Uint64(b[pos:])), 'g', -1, 64)
			pos += 8
		case binInt16:
			if pos+2 > len(b) {
				return nil, truncated()
			}
			value = strconv.Itoa(int(int16(binary.LittleEndian.Uint16(b[pos:]))))
			pos += 2
		case binInt32:
			if pos+4 > len(b) {
				return nil, truncated()
			}
			value = strconv.Itoa(int(int32(binary.LittleEndian.Uint32(b[pos:]))))
			pos += 4
		case binInt64:
			if pos+8 > len(b) {
				return nil, truncated()
			}
			value = strconv.FormatInt(int64(binary.LittleEndian.Uint64(b[pos:])), 10)
			pos += 8
		case binBool:
			if pos+1 > len(b) {
				return nil, truncated()
			}
			value = strconv.Itoa(int(b[pos]))
			pos++
		case binChunk:
			if pos+1 > len(b) {
				return nil, truncated()
			}
			n := int(b[pos])
			if pos+1+n > len(b) {
				return nil, truncated()
			}
			value = fmt.Sprintf("%X", b[pos+1:pos+1+n])
			pos += 1 + n
		default:
			end := bytes.IndexByte(b[pos:], 0)
			if end < 0 {
				return nil, truncated()
			}
			value = string(b[pos : pos+end])
			pos += end + 1
		}
		tags = append(tags, tag{code: code, value: value, line: line})
		if code == 0 && value == "EOF" {
			break
		}
	}
	return tags, nil
}

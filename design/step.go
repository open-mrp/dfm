package design

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// A STEP file (ISO 10303-21) is a list of numbered entity instances, each a type name with parameters, or a complex instance holding several typed parts. stepFile holds them by number; geometry is read from it in stepread.go.

type stepKind int

const (
	stepUnset stepKind = iota // $ or *
	stepRef
	stepNumber
	stepString
	stepEnum
	stepList
	stepTyped // a typed value such as LENGTH_MEASURE(25.4)
)

type stepValue struct {
	kind stepKind
	ref  int
	num  float64
	str  string // string, enum or typed value's type
	list []stepValue
}

// stepPart is one typed part of an instance: the whole of a simple one.
type stepPart struct {
	name   string
	params []stepValue
}

type stepEntity struct {
	id    int
	parts []stepPart
}

// param returns parameter i, or an unset value when the part has no such parameter: a malformed file reads as missing data, never out of range.
func (p stepPart) param(i int) stepValue {
	if i < 0 || i >= len(p.params) {
		return stepValue{}
	}
	return p.params[i]
}

// part returns the entity's part of the named type.
func (e *stepEntity) part(name string) (stepPart, bool) {
	for _, p := range e.parts {
		if p.name == name {
			return p, true
		}
	}
	return stepPart{}, false
}

// is reports whether the entity has a part of the named type.
func (e *stepEntity) is(name string) bool {
	_, ok := e.part(name)
	return ok
}

// name is the type of a simple entity, or "" for a complex one.
func (e *stepEntity) name() string {
	if len(e.parts) == 1 {
		return e.parts[0].name
	}
	return ""
}

type stepFile struct {
	entities map[int]*stepEntity
	order    []int
}

// parseStepFile reads the DATA sections of a STEP file.
func parseStepFile(data []byte) (*stepFile, error) {
	if !bytes.HasPrefix(bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n"), []byte("ISO-10303-21")) {
		return nil, fmt.Errorf("%w: not a STEP file (no ISO-10303-21 header)", ErrInvalid)
	}
	s := &stepScanner{src: data}
	f := &stepFile{entities: map[int]*stepEntity{}}
	inData := false
	for {
		s.skipSpace()
		if s.pos >= len(s.src) {
			break
		}
		if !inData {
			if s.src[s.pos] == '\'' {
				s.str()
				continue
			}
			kw := s.keyword()
			if kw == "" {
				s.pos++
				continue
			}
			if kw == "DATA" {
				inData = true
				s.skipTo(';')
			}
			continue
		}
		c := s.src[s.pos]
		if c != '#' {
			kw := s.keyword()
			if kw == "ENDSEC" {
				inData = false
				s.skipTo(';')
				continue
			}
			return nil, s.errorf("expected an entity instance")
		}
		s.pos++
		id, ok := s.integer()
		if !ok {
			return nil, s.errorf("bad entity number")
		}
		s.skipSpace()
		if !s.consume('=') {
			return nil, s.errorf("expected = after #%d", id)
		}
		e := &stepEntity{id: id}
		s.skipSpace()
		if s.consume('(') {
			for {
				s.skipSpace()
				if s.consume(')') {
					break
				}
				p, err := s.part()
				if err != nil {
					return nil, err
				}
				e.parts = append(e.parts, p)
			}
		} else {
			p, err := s.part()
			if err != nil {
				return nil, err
			}
			e.parts = append(e.parts, p)
		}
		s.skipSpace()
		if !s.consume(';') {
			return nil, s.errorf("expected ; after #%d", id)
		}
		if len(e.parts) == 0 {
			return nil, s.errorf("entity #%d has no type", id)
		}
		if _, dup := f.entities[id]; dup {
			return nil, s.errorf("entity #%d defined twice", id)
		}
		f.entities[id] = e
		f.order = append(f.order, id)
	}
	if len(f.entities) == 0 {
		return nil, fmt.Errorf("%w: the STEP file has no data", ErrInvalid)
	}
	return f, nil
}

type stepScanner struct {
	src []byte
	pos int
}

func (s *stepScanner) errorf(format string, args ...any) error {
	line := bytes.Count(s.src[:min(s.pos, len(s.src))], []byte("\n")) + 1
	return fmt.Errorf("%w: STEP line %d: %s", ErrInvalid, line, fmt.Sprintf(format, args...))
}

// skipSpace skips white space and comments.
func (s *stepScanner) skipSpace() {
	for s.pos < len(s.src) {
		switch c := s.src[s.pos]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			s.pos++
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*':
			end := bytes.Index(s.src[s.pos+2:], []byte("*/"))
			if end < 0 {
				s.pos = len(s.src)
				return
			}
			s.pos += end + 4
		default:
			return
		}
	}
}

// skipTo moves past the next c outside strings.
func (s *stepScanner) skipTo(c byte) {
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case '\'':
			s.str()
			continue
		case c:
			s.pos++
			return
		}
		s.pos++
	}
}

func (s *stepScanner) consume(c byte) bool {
	if s.pos < len(s.src) && s.src[s.pos] == c {
		s.pos++
		return true
	}
	return false
}

func (s *stepScanner) keyword() string {
	start := s.pos
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		letter := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
		later := s.pos > start && ((c >= '0' && c <= '9') || c == '-')
		if letter || later {
			s.pos++
			continue
		}
		break
	}
	return strings.ToUpper(string(s.src[start:s.pos]))
}

func (s *stepScanner) integer() (int, bool) {
	start := s.pos
	for s.pos < len(s.src) && s.src[s.pos] >= '0' && s.src[s.pos] <= '9' {
		s.pos++
	}
	n, err := strconv.Atoi(string(s.src[start:s.pos]))
	return n, err == nil
}

// str reads a quoted string, ” standing for a quote.
func (s *stepScanner) str() string {
	s.pos++ // opening quote
	var b strings.Builder
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		s.pos++
		if c == '\'' {
			if s.pos < len(s.src) && s.src[s.pos] == '\'' {
				b.WriteByte('\'')
				s.pos++
				continue
			}
			return b.String()
		}
		b.WriteByte(c)
	}
	return b.String()
}

// part reads NAME(params).
func (s *stepScanner) part() (stepPart, error) {
	s.skipSpace()
	name := s.keyword()
	if name == "" {
		return stepPart{}, s.errorf("expected an entity type")
	}
	s.skipSpace()
	if !s.consume('(') {
		return stepPart{}, s.errorf("expected ( after %s", name)
	}
	params, err := s.list()
	return stepPart{name: name, params: params}, err
}

// list reads parameters up to the closing parenthesis.
func (s *stepScanner) list() ([]stepValue, error) {
	var out []stepValue
	for {
		s.skipSpace()
		if s.consume(')') {
			return out, nil
		}
		if len(out) > 0 {
			if !s.consume(',') {
				return nil, s.errorf("expected , or )")
			}
			s.skipSpace()
		}
		v, err := s.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (s *stepScanner) value() (stepValue, error) {
	if s.pos >= len(s.src) {
		return stepValue{}, s.errorf("unexpected end of file")
	}
	switch c := s.src[s.pos]; {
	case c == '$' || c == '*':
		s.pos++
		return stepValue{kind: stepUnset}, nil
	case c == '#':
		s.pos++
		n, ok := s.integer()
		if !ok {
			return stepValue{}, s.errorf("bad reference")
		}
		return stepValue{kind: stepRef, ref: n}, nil
	case c == '\'':
		return stepValue{kind: stepString, str: s.str()}, nil
	case c == '"': // binary
		end := bytes.IndexByte(s.src[s.pos+1:], '"')
		if end < 0 {
			return stepValue{}, s.errorf("unterminated binary")
		}
		s.pos += end + 2
		return stepValue{kind: stepUnset}, nil
	case c == '.':
		end := bytes.IndexByte(s.src[s.pos+1:], '.')
		if end < 0 {
			return stepValue{}, s.errorf("unterminated enumeration")
		}
		v := stepValue{kind: stepEnum, str: string(s.src[s.pos+1 : s.pos+1+end])}
		s.pos += end + 2
		return v, nil
	case c == '(':
		s.pos++
		l, err := s.list()
		return stepValue{kind: stepList, list: l}, err
	case c == '-' || c == '+' || c >= '0' && c <= '9':
		start := s.pos
		s.pos++
		for s.pos < len(s.src) {
			c := s.src[s.pos]
			if c >= '0' && c <= '9' || c == '.' || c == 'E' || c == 'e' || (c == '-' || c == '+') && (s.src[s.pos-1] == 'E' || s.src[s.pos-1] == 'e') {
				s.pos++
				continue
			}
			break
		}
		f, err := strconv.ParseFloat(string(s.src[start:s.pos]), 64)
		if err != nil {
			return stepValue{}, s.errorf("bad number %q", s.src[start:s.pos])
		}
		return stepValue{kind: stepNumber, num: f}, nil
	default:
		name := s.keyword()
		if name == "" {
			return stepValue{}, s.errorf("unexpected %q", c)
		}
		s.skipSpace()
		if !s.consume('(') {
			return stepValue{}, s.errorf("expected ( after %s", name)
		}
		l, err := s.list()
		return stepValue{kind: stepTyped, str: name, list: l}, err
	}
}

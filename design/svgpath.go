package design

import (
	"fmt"
	"math"
	"strconv"
)

// pathOp is one drawing step of SVG path data, in absolute user coordinates: a line, a cubic Bézier (quadratics are raised to cubics), an elliptical arc, or the start of a subpath (move) or its closing (close).
type pathOp struct {
	kind byte // 'M', 'L', 'C', 'A', 'Z'
	// to is the end point; c1, c2 the cubic's control points.
	to, c1, c2 [2]float64
	// Arc parameters.
	rx, ry, rotation float64
	large, sweep     bool
}

// parsePathData reads SVG path data ("d"), resolving relative and shorthand commands to absolute lines, cubics and arcs.
func parsePathData(d string) ([]pathOp, error) {
	s := &pathScanner{s: d}
	var ops []pathOp
	var cur, start, lastCtrl [2]float64
	var lastCmd byte
	var cmd byte
	for {
		s.skipSeparators()
		if s.done() {
			return ops, nil
		}
		if c := s.s[s.i]; isCommand(c) {
			cmd = c
			s.i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("path data must start with a command, at %q", s.rest())
		} else if cmd == 'M' {
			cmd = 'L' // coordinates after a moveto are lines
		} else if cmd == 'm' {
			cmd = 'l'
		} else if cmd == 'Z' || cmd == 'z' {
			return nil, fmt.Errorf("numbers after a closepath, at %q", s.rest())
		}
		rel := cmd >= 'a' && cmd <= 'z'
		base := [2]float64{}
		if rel {
			base = cur
		}
		up := cmd &^ 0x20
		switch up {
		case 'Z':
			ops = append(ops, pathOp{kind: 'Z', to: start})
			cur = start
		case 'M', 'L', 'T':
			x, y, err := s.pair()
			if err != nil {
				return nil, err
			}
			p := [2]float64{base[0] + x, base[1] + y}
			if up == 'M' {
				ops = append(ops, pathOp{kind: 'M', to: p})
				start = p
			} else if up == 'L' {
				ops = append(ops, pathOp{kind: 'L', to: p})
			} else { // smooth quadratic: reflect the last quadratic control point
				q := cur
				if lastCmd == 'Q' || lastCmd == 'T' {
					q = [2]float64{2*cur[0] - lastCtrl[0], 2*cur[1] - lastCtrl[1]}
				}
				ops = append(ops, quadToCubic(cur, q, p))
				lastCtrl = q
			}
			cur = p
		case 'H', 'V':
			v, err := s.number()
			if err != nil {
				return nil, err
			}
			p := cur
			if up == 'H' {
				p[0] = base[0] + v
				if !rel {
					p[0] = v
				}
			} else {
				p[1] = base[1] + v
				if !rel {
					p[1] = v
				}
			}
			ops = append(ops, pathOp{kind: 'L', to: p})
			cur = p
		case 'C', 'S':
			var c1 [2]float64
			if up == 'C' {
				x, y, err := s.pair()
				if err != nil {
					return nil, err
				}
				c1 = [2]float64{base[0] + x, base[1] + y}
			} else if lastCmd == 'C' || lastCmd == 'S' {
				c1 = [2]float64{2*cur[0] - lastCtrl[0], 2*cur[1] - lastCtrl[1]}
			} else {
				c1 = cur
			}
			x2, y2, err := s.pair()
			if err != nil {
				return nil, err
			}
			x, y, err := s.pair()
			if err != nil {
				return nil, err
			}
			c2 := [2]float64{base[0] + x2, base[1] + y2}
			p := [2]float64{base[0] + x, base[1] + y}
			ops = append(ops, pathOp{kind: 'C', c1: c1, c2: c2, to: p})
			lastCtrl, cur = c2, p
		case 'Q':
			qx, qy, err := s.pair()
			if err != nil {
				return nil, err
			}
			x, y, err := s.pair()
			if err != nil {
				return nil, err
			}
			q := [2]float64{base[0] + qx, base[1] + qy}
			p := [2]float64{base[0] + x, base[1] + y}
			ops = append(ops, quadToCubic(cur, q, p))
			lastCtrl, cur = q, p
		case 'A':
			var v [3]float64
			for k := range v {
				f, err := s.number()
				if err != nil {
					return nil, err
				}
				v[k] = f
			}
			large, err := s.flag()
			if err != nil {
				return nil, err
			}
			sweep, err := s.flag()
			if err != nil {
				return nil, err
			}
			x, y, err := s.pair()
			if err != nil {
				return nil, err
			}
			p := [2]float64{base[0] + x, base[1] + y}
			ops = append(ops, pathOp{kind: 'A', rx: math.Abs(v[0]), ry: math.Abs(v[1]), rotation: v[2], large: large, sweep: sweep, to: p})
			cur = p
		default:
			return nil, fmt.Errorf("unknown path command %q", cmd)
		}
		lastCmd = up
	}
}

func isCommand(c byte) bool {
	switch c &^ 0x20 {
	case 'M', 'Z', 'L', 'H', 'V', 'C', 'S', 'Q', 'T', 'A':
		return true
	}
	return false
}

// quadToCubic raises a quadratic Bézier from p0 via q to p to the equivalent cubic.
func quadToCubic(p0, q, p [2]float64) pathOp {
	return pathOp{
		kind: 'C',
		// Each product is converted before it is added, so no CPU fuses them (see AGENTS.md).
		c1: [2]float64{p0[0] + float64(2.0/3*(q[0]-p0[0])), p0[1] + float64(2.0/3*(q[1]-p0[1]))},
		c2: [2]float64{p[0] + float64(2.0/3*(q[0]-p[0])), p[1] + float64(2.0/3*(q[1]-p[1]))},
		to: p,
	}
}

// pathScanner reads numbers, flags and separators from path data and other number lists (points, viewBox, transforms).
type pathScanner struct {
	s string
	i int
}

func (s *pathScanner) done() bool   { return s.i >= len(s.s) }
func (s *pathScanner) rest() string { return truncate(s.s[s.i:]) }

func (s *pathScanner) skipSeparators() {
	for s.i < len(s.s) {
		switch s.s[s.i] {
		case ' ', '\t', '\n', '\r', '\f', ',':
			s.i++
		default:
			return
		}
	}
}

// number reads one number, which may run straight on from the previous one ("1-2", ".5.5", "1e-3").
func (s *pathScanner) number() (float64, error) {
	s.skipSeparators()
	start := s.i
	if s.i < len(s.s) && (s.s[s.i] == '+' || s.s[s.i] == '-') {
		s.i++
	}
	digits, dot := false, false
	for s.i < len(s.s) {
		c := s.s[s.i]
		switch {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' && !dot:
			dot = true
		default:
			goto exponent
		}
		s.i++
	}
exponent:
	if digits && s.i < len(s.s) && (s.s[s.i] == 'e' || s.s[s.i] == 'E') {
		j := s.i + 1
		if j < len(s.s) && (s.s[j] == '+' || s.s[j] == '-') {
			j++
		}
		if j < len(s.s) && s.s[j] >= '0' && s.s[j] <= '9' {
			s.i = j
			for s.i < len(s.s) && s.s[s.i] >= '0' && s.s[s.i] <= '9' {
				s.i++
			}
		}
	}
	if !digits {
		s.i = start
		return 0, fmt.Errorf("expected a number at %q", s.rest())
	}
	v, err := strconv.ParseFloat(s.s[start:s.i], 64)
	if err != nil || math.IsInf(v, 0) || math.Abs(v) > maxMagnitude {
		return 0, fmt.Errorf("number %q is out of range", s.s[start:s.i])
	}
	return v, nil
}

func (s *pathScanner) pair() (float64, float64, error) {
	x, err := s.number()
	if err != nil {
		return 0, 0, err
	}
	y, err := s.number()
	return x, y, err
}

// flag reads an arc flag, a single 0 or 1 that may touch what follows.
func (s *pathScanner) flag() (bool, error) {
	s.skipSeparators()
	if s.i < len(s.s) && (s.s[s.i] == '0' || s.s[s.i] == '1') {
		s.i++
		return s.s[s.i-1] == '1', nil
	}
	return false, fmt.Errorf("expected an arc flag (0 or 1) at %q", s.rest())
}

// numbers reads a whole list of numbers.
func numbers(list string) ([]float64, error) {
	s := &pathScanner{s: list}
	var out []float64
	for {
		s.skipSeparators()
		if s.done() {
			return out, nil
		}
		v, err := s.number()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

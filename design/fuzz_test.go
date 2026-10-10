package design

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-mrp/dfm/constants"
)

// FuzzParseDXF ensures arbitrary input never panics and that every failure is one of the documented sentinel errors.
func FuzzParseDXF(f *testing.F) {
	paths, _ := filepath.Glob("../fixtures/*/*.dxf") // includes binary DXF
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil && len(data) < 4096 {
			f.Add(data)
		}
	}
	f.Add([]byte(dxf("", circleEnt)))
	f.Add([]byte(dxf("", "0\nLWPOLYLINE\n90\n2\n70\n1\n10\n0\n20\n0\n42\n1\n10\n1\n20\n0\n42\n1\n")))
	f.Add([]byte(dxf("", "0\nPOLYLINE\n70\n1\n0\nVERTEX\n10\n0\n20\n0\n42\n-1\n0\nVERTEX\n10\n2\n20\n0\n0\nSEQEND\n")))
	// Block references: placed, arrayed, nested and stretched.
	f.Add([]byte(withBlocks(dxf("", "0\nINSERT\n5\nI\n2\nB\n10\n5\n20\n5\n41\n2\n42\n1\n50\n30\n70\n2\n71\n2\n44\n9\n45\n9\n"),
		blockDef("B", 1, 1, 0, circleEnt, "0\nARC\n10\n0\n20\n0\n40\n3\n50\n0\n51\n180\n", "0\nINSERT\n2\nC\n10\n0\n20\n0\n"),
		blockDef("C", 0, 0, 0, "0\nLINE\n10\n0\n20\n0\n11\n1\n21\n1\n"))))
	// Binary DXF with two-byte and one-byte group codes.
	f.Add(toBinary(f, []byte(dxf("", circleEnt, "0\nLWPOLYLINE\n90\n2\n70\n1\n10\n0\n20\n0\n42\n1\n10\n1\n20\n0\n")), false))
	f.Add(toBinary(f, []byte(dxf("", circleEnt)), true))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParseDXF(data, "")
		if err != nil {
			if !isSentinel(err) {
				t.Fatalf("undocumented error: %v", err)
			}
			return
		}
		_ = p.Holes()
	})
}

func isSentinel(err error) bool {
	for _, s := range []error{ErrInvalid, ErrUnsupportedContent, ErrUnitsUnknown} {
		if errors.Is(err, s) {
			return true
		}
	}
	return false
}

// FuzzParseSVG ensures arbitrary SVG never panics and that every failure is one of the documented sentinel errors.
func FuzzParseSVG(f *testing.F) {
	paths, _ := filepath.Glob("../fixtures/*/*.svg")
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte(svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, `<path d="M0 0a5 3 30 1 0 5 5q2 2 3 0t2 0s1 1 2 0z" stroke="red" transform="skewX(10) rotate(20 5 5)"/>`)))
	f.Add([]byte(svgDoc(`width="1in" height="1in" viewBox="0 0 10 10" preserveAspectRatio="xMinYMax slice"`, `<defs><g id="a"><use href="#b"/></g><g id="b"><circle r="2" stroke="black"/></g></defs><use href="#a" x="3"/>`)))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParseSVG(data, "")
		if err != nil {
			if !isSentinel(err) {
				t.Fatalf("undocumented error: %v", err)
			}
			return
		}
		_ = p.Holes()
	})
}

// FuzzParseSTEP checks the STEP reader never panics or hangs on malformed input: every problem is an error.
func FuzzParseSTEP(f *testing.F) {
	paths, _ := filepath.Glob("../fixtures/*/*.step")
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte("ISO-10303-21;\nDATA;\n#1=CARTESIAN_POINT('');\n#2=MANIFOLD_SOLID_BREP('',#1);\nENDSEC;\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseModel(Design{Format: constants.DesignFormatSTEP, Data: data})
	})
}

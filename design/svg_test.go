package design

import (
	"errors"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

func TestParsePathData(t *testing.T) {
	tests := []struct {
		d    string
		want []pathOp
	}{
		// Relative moves, implicit lines after a move, H and V, numbers run together.
		{"m10 10 5-5h10v10z", []pathOp{
			{kind: 'M', to: [2]float64{10, 10}}, {kind: 'L', to: [2]float64{15, 5}},
			{kind: 'L', to: [2]float64{25, 5}}, {kind: 'L', to: [2]float64{25, 15}}, {kind: 'Z', to: [2]float64{10, 10}},
		}},
		{"M0,0L.5.5 1e1-1", []pathOp{{kind: 'M'}, {kind: 'L', to: [2]float64{.5, .5}}, {kind: 'L', to: [2]float64{10, -1}}}},
		// Arc flags may touch each other and the next number.
		{"M0 0a5 5 0 01 10 0", []pathOp{{kind: 'M'}, {kind: 'A', rx: 5, ry: 5, sweep: true, to: [2]float64{10, 0}}}},
		// A smooth cubic reflects the previous control point.
		{"M0 0C0 10 10 10 10 0S20-10 20 0", []pathOp{
			{kind: 'M'},
			{kind: 'C', c1: [2]float64{0, 10}, c2: [2]float64{10, 10}, to: [2]float64{10, 0}},
			{kind: 'C', c1: [2]float64{10, -10}, c2: [2]float64{20, -10}, to: [2]float64{20, 0}},
		}},
	}
	for _, tt := range tests {
		got, err := parsePathData(tt.d)
		if err != nil {
			t.Errorf("%q: %v", tt.d, err)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%q:\n got %+v\nwant %+v", tt.d, got, tt.want)
		}
	}
	for _, bad := range []string{"10 10", "M0 0 L", "M0 0 A1 1 0 2 1 5 5", "M0 0 X1"} {
		if _, err := parsePathData(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// svgDoc wraps body in an SVG root with the given attributes.
func svgDoc(rootAttrs, body string) string {
	return `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" ` + rootAttrs + `>` + body + `</svg>`
}

const square10 = `<rect id="sq" x="0" y="0" width="10" height="10" stroke="#000" fill="none"/>`

func TestSVGUnits(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		units   constants.LengthUnit
		width   float64 // of the 10-unit square, in mm
		drawn   constants.LengthUnit
		assumed bool
	}{
		{"mm viewBox", svgDoc(`width="100mm" height="50mm" viewBox="0 0 100 50"`, square10), "", 10, constants.LengthUnitMillimeter, false},
		{"viewBox in other units", svgDoc(`width="100mm" height="50mm" viewBox="0 0 400 200"`, square10), "", 2.5, constants.LengthUnitMillimeter, false},
		{"points", svgDoc(`width="72pt" height="72pt" viewBox="0 0 72 72"`, square10), "", 10 * 25.4 / 72, constants.LengthUnitInch, false},
		{"inches, mm user units", svgDoc(`width="12in" height="12in" viewBox="0 0 304.8 304.8"`, square10), "", 10, constants.LengthUnitInch, false},
		{"pixels at 96", svgDoc(`width="96" height="96" viewBox="0 0 96 96"`, square10), "", 10 * 25.4 / 96, "", true},
		{"no viewBox", svgDoc(`width="100mm" height="50mm"`, square10), "", 10 * 25.4 / 96, constants.LengthUnitMillimeter, true},
		{"override", svgDoc(`width="96" height="96" viewBox="0 0 96 96"`, square10), constants.LengthUnitInch, 254, constants.LengthUnitInch, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseSVG([]byte(tt.doc), tt.units)
			if err != nil {
				t.Fatal(err)
			}
			if w := p.Bounds.WidthMM(); math.Abs(w-tt.width) > 1e-9 {
				t.Errorf("square is %g mm wide, want %g", w, tt.width)
			}
			if p.Units != tt.drawn || p.UnitsAssumed != tt.assumed {
				t.Errorf("units %q assumed %v, want %q %v", p.Units, p.UnitsAssumed, tt.drawn, tt.assumed)
			}
		})
	}
	// Pixels follow the program that wrote the file.
	for _, tt := range []struct {
		name, head string
		px         float64
	}{
		{"Illustrator", "<!-- Generator: Adobe Illustrator 18.0.0, SVG Export Plug-In -->", 25.4 / 72},
		{"old Inkscape", "", 25.4 / 90},
	} {
		doc := `<?xml version="1.0"?>` + tt.head + `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100" viewBox="0 0 100 100"`
		if tt.name == "old Inkscape" {
			doc += ` inkscape:version="0.91 r13725"`
		}
		doc += `>` + square10 + `</svg>`
		p, err := ParseSVG([]byte(doc), "")
		if err != nil {
			t.Fatal(err)
		}
		if w := p.Bounds.WidthMM(); math.Abs(w-10*tt.px) > 1e-9 {
			t.Errorf("%s: square is %g mm wide, want %g", tt.name, w, 10*tt.px)
		}
	}
}

func TestSVGTransforms(t *testing.T) {
	doc := svgDoc(`width="100mm" height="100mm" viewBox="0 0 100 100"`,
		`<g transform="translate(50 50) rotate(90)"><line id="l" x1="0" y1="0" x2="10" y2="0" stroke="black"/></g>`+
			`<g transform="matrix(1 0 0 1 0 20)"><line id="m" x1="0" y1="0" x2="5" y2="0" stroke="black"/></g>`)
	p, err := ParseSVG([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]geometry.Point{}
	for _, c := range p.Contours {
		s := c.Segments[0]
		got[s.EntityID] = [2]geometry.Point{s.Start, s.End}
	}
	if s := got["l"]; !near(s[0], geometry.Point{X: 50, Y: 50}) || !near(s[1], geometry.Point{X: 50, Y: 60}) {
		t.Errorf("rotated line %v, want (50,50) to (50,60)", s)
	}
	if s := got["m"]; !near(s[0], geometry.Point{Y: 20}) || !near(s[1], geometry.Point{X: 5, Y: 20}) {
		t.Errorf("matrix line %v", s)
	}
}

func TestSVGOperations(t *testing.T) {
	doc := svgDoc(`xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="200mm" height="200mm" viewBox="0 0 200 200"`, `
<style>.cut { stroke: #0000ff; fill: none } #hid { display: none }</style>
<rect id="plate" x="0" y="0" width="150" height="150" class="cut"/>
<circle id="hole" cx="20" cy="20" r="5" class="cut"/>
<rect id="fillonly" x="40" y="40" width="10" height="10" fill="#f6921e"/>
<rect id="hid" x="60" y="40" width="10" height="10" stroke="black" fill="none"/>
<rect id="invisible" x="80" y="40" width="10" height="10" fill="none"/>
<g stroke="black" fill="none"><circle id="inherited" cx="100" cy="100" r="4"/></g>
<g inkscape:groupmode="layer" inkscape:label="Engrave text"><path id="engraved" d="M10 120 L30 120" stroke="red"/></g>
<g style="opacity:0"><circle id="faded" cx="120" cy="20" r="3" stroke="black"/></g>
<text id="label" x="5" y="140">PART-7</text>
<image id="logo" x="60" y="60" width="10" height="10" href="data:image/png;base64,AAAA"/>`)
	p, err := ParseSVG([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	var cut []string
	for _, c := range p.Contours {
		cut = append(cut, c.Segments[0].EntityID)
	}
	for _, c := range p.Circles {
		cut = append(cut, c.EntityIDs[0])
	}
	if !slices.Equal(cut, []string{"plate", "hole", "inherited"}) {
		t.Errorf("cut %v, want plate, hole, inherited", cut)
	}
	kinds := map[string]geometry.NonCutKind{}
	for _, n := range p.NonCut {
		kinds[n.EntityID] = n.Kind
	}
	want := map[string]geometry.NonCutKind{
		"fillonly": geometry.NonCutEngrave, "hid": geometry.NonCutIgnored, "invisible": geometry.NonCutIgnored,
		"engraved": geometry.NonCutEngrave, "faded": geometry.NonCutIgnored, "logo": geometry.NonCutEngrave,
	}
	if len(kinds) != len(want) {
		t.Errorf("non-cut %v, want %v", kinds, want)
	}
	for id, k := range want {
		if kinds[id] != k {
			t.Errorf("%s: kind %v, want %v", id, kinds[id], k)
		}
	}
	if len(p.Text) != 1 || p.Text[0].EntityID != "label" || !near(*p.Text[0].Position, geometry.Point{X: 5, Y: 140}) {
		t.Errorf("text %+v", p.Text)
	}
}

func TestSVGColorOperations(t *testing.T) {
	doc := svgDoc(`width="200mm" height="200mm" viewBox="0 0 200 200"`, `
<rect id="plate" x="0" y="0" width="150" height="150" stroke="#0000FF" fill="none"/>
<circle id="etched" cx="20" cy="20" r="5" stroke="red" fill="none"/>
<circle id="guide" cx="40" cy="20" r="5" stroke="rgb(0,255,0)" fill="none"/>
<circle id="filled" cx="60" cy="20" r="5" fill="#aa0000"/>
<circle id="grey" cx="80" cy="20" r="5" fill="#808080"/>`)
	ops := Operations{
		ByColor: map[string]constants.ShapeOperation{"#ff0000": constants.ShapeOperationEngrave, "#00ff00": constants.ShapeOperationIgnore, "#808080": constants.ShapeOperationEngrave},
		Filled:  constants.ShapeOperationCut,
	}
	p, err := parseSVG([]byte(doc), "", ops, nil, geometry.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var cut []string
	for _, c := range p.Contours {
		cut = append(cut, c.Segments[0].EntityID)
	}
	for _, c := range p.Circles {
		cut = append(cut, c.EntityIDs[0])
	}
	if !slices.Equal(cut, []string{"plate", "filled"}) {
		t.Errorf("cut %v, want plate, filled", cut)
	}
	kinds := map[string]geometry.NonCutKind{}
	for _, n := range p.NonCut {
		kinds[n.EntityID] = n.Kind
	}
	want := map[string]geometry.NonCutKind{"etched": geometry.NonCutEngrave, "guide": geometry.NonCutIgnored, "grey": geometry.NonCutEngrave}
	if !maps.Equal(kinds, want) {
		t.Errorf("non-cut %v, want %v", kinds, want)
	}
}

func TestSVGShapes(t *testing.T) {
	doc := svgDoc(`width="200mm" height="200mm" viewBox="0 0 200 200"`, `
<g stroke="black" fill="none">
<rect id="rounded" x="0" y="0" width="40" height="20" rx="5"/>
<path id="arc" d="M60 10 A10 10 0 0 1 80 10 L80 30 L60 30 Z"/>
<polygon id="tri" points="100,0 120,0 110,15"/>
<ellipse id="oval" cx="150" cy="50" rx="20" ry="10"/>
<g transform="scale(2 1)"><circle id="stretched" cx="20" cy="100" r="10"/></g>
</g>`)
	p, err := ParseSVG([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	bounds := map[string]geometry.Bounds{}
	arcs := map[string]int{}
	for _, c := range p.Contours {
		id := c.Segments[0].EntityID
		if !c.Closed {
			t.Errorf("%s is open", id)
		}
		bounds[id] = c.Bounds()
		for _, s := range c.Segments {
			if s.IsArc() {
				arcs[id]++
			}
		}
	}
	check := func(id string, w, h float64) {
		b, ok := bounds[id]
		if !ok || math.Abs(b.WidthMM()-w) > 1e-3 || math.Abs(b.HeightMM()-h) > 1e-3 {
			t.Errorf("%s: %gx%g, want %gx%g", id, b.WidthMM(), b.HeightMM(), w, h)
		}
	}
	check("rounded", 40, 20)
	check("arc", 20, 30) // the arc bulges 10 above y=10
	check("tri", 20, 15)
	check("oval", 40, 20)
	check("stretched", 40, 20) // a circle scaled 2x along X
	if arcs["rounded"] != 4 || arcs["arc"] != 1 {
		t.Errorf("exact arcs: rounded %d, arc %d; want 4 and 1", arcs["rounded"], arcs["arc"])
	}
}

func TestSVGUse(t *testing.T) {
	doc := svgDoc(`xmlns:xlink="http://www.w3.org/1999/xlink" width="200mm" height="200mm" viewBox="0 0 200 200"`, `
<defs><circle id="hole" cx="0" cy="0" r="3" stroke="black" fill="none"/>
<symbol id="pair"><use id="u1" xlink:href="#hole" x="0" y="0"/><use id="u2" href="#hole" x="10" y="0"/></symbol></defs>
<rect id="plate" x="0" y="0" width="100" height="100" stroke="black" fill="none"/>
<use id="a" href="#hole" x="20" y="20"/>
<use id="b" href="#pair" x="50" y="50"/>`)
	p, err := ParseSVG([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]geometry.Point{}
	for _, c := range p.Circles {
		got[c.EntityIDs[0]] = c.Center
	}
	want := map[string]geometry.Point{"a/hole": {X: 20, Y: 20}, "b/u1/hole": {X: 50, Y: 50}, "b/u2/hole": {X: 60, Y: 50}}
	if len(got) != len(want) {
		t.Fatalf("circles %v, want %v", got, want)
	}
	for id, c := range want {
		if !near(got[id], c) {
			t.Errorf("%s at %v, want %v", id, got[id], c)
		}
	}
	if _, ok := got["hole"]; ok {
		t.Error("a definition was drawn on its own")
	}
}

func TestSVGErrors(t *testing.T) {
	tests := []struct {
		name, doc string
		want      error
	}{
		{"not svg", `<html><body/></html>`, ErrInvalid},
		{"broken xml", `<svg><rect`, ErrInvalid},
		{"no cut geometry", svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, `<rect width="5" height="5" fill="red"/>`), ErrInvalid},
		{"undefined use", svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, square10+`<use href="#nope"/>`), ErrInvalid},
		{"circular use", svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, square10+`<g id="loop"><use href="#loop"/></g>`), ErrInvalid},
		{"bad path", svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, `<path d="10 10" stroke="black"/>`), ErrInvalid},
		{"unknown width unit", svgDoc(`width="10em" height="10mm" viewBox="0 0 10 10"`, square10), ErrUnsupportedContent},
	}
	for _, tt := range tests {
		if _, err := ParseSVG([]byte(tt.doc), ""); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := ParseSVG([]byte(svgDoc(`width="10mm" height="10mm" viewBox="0 0 10 10"`, `<rect width="5" height="5" fill="red"/>`)), ""); err == nil || !strings.Contains(err.Error(), "only stroked shapes are cut") {
		t.Errorf("no-cut message: %v", err)
	}
}

func TestSVGEllipticalArcs(t *testing.T) {
	// Arcs drawn either way round, rotated or not, and a near-circle whose radii differ by rounding: each becomes a path from its start to its end.
	for _, d := range []string{
		"M 0,0 A 54.9,30 0 0 0 60,10",
		"M 0,0 A 54.9,30 67.5 0 0 60,10",
		"M 0,0 A 54.9,30 67.5 1 1 60,10",
		"M 0,0 A 54.903763,54.903755 67.5 0 0 69.37241,-6.73546",
	} {
		doc := svgDoc(`width="200mm" height="200mm" viewBox="-100 -100 200 200"`, `<path d="`+d+`" stroke="blue" fill="none"/>`)
		p, err := ParseSVG([]byte(doc), "")
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		segs := p.Contours[0].Segments
		start, end := segs[0].Start, segs[len(segs)-1].End
		if start.Dist(geometry.Point{X: 100, Y: 100}) > 1e-6 || math.Abs(end.X-start.X) < 1 {
			t.Errorf("%s: path runs %v to %v", d, start, end)
		}
	}
}

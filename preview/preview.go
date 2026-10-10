// Package preview draws a part as the engine read it, with its findings marked, as SVG: so a person or an agent can confirm the file was read as intended and see where each problem is.
package preview

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/rules"
)

// Render returns an SVG of the part's cut paths (closed paths black, open ones orange, holes blue), its dashed bend lines, and a marker at each finding's location (red for errors, amber for warnings) with its measured span, each marker titled with the finding's code and message. Coordinates are millimeters; the drawing's Y axis points up. Elements carry classes (cut, open, hole, bend, finding error, finding warning, span) for restyling.
func Render(part *geometry.Part, findings []rules.Finding) []byte {
	b := part.Bounds
	if b.IsEmpty() {
		b = geometry.Bounds{MaxX: 1, MaxY: 1}
	}
	w, h := math.Max(b.WidthMM(), 1e-3), math.Max(b.HeightMM(), 1e-3)
	pad := math.Max(w, h) * 0.04
	marker := math.Max(w, h) / 120
	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%smm" height="%smm">`,
		num(b.MinX-pad), num(-b.MaxY-pad), num(w+2*pad), num(h+2*pad), num(w+2*pad), num(h+2*pad))
	s.WriteString(`<style>path,circle,line{fill:none;vector-effect:non-scaling-stroke;stroke-width:1}` +
		`.cut{stroke:#000}.open{stroke:#e67e00}.hole{stroke:#1f5fbf}.bend{stroke:#1f5fbf;stroke-dasharray:4 3}` +
		`.finding.error{stroke:#d00;stroke-width:2}.finding.warning{stroke:#e0a000;stroke-width:2}.span{stroke:#d00}</style>`)
	s.WriteString(`<g transform="scale(1,-1)">`)
	for _, c := range part.Contours {
		class := "cut"
		if !c.Closed {
			class = "open"
		}
		fmt.Fprintf(&s, `<path class="%s" d="%s"><title>%s</title></path>`, class, pathData(c), html.EscapeString(strings.Join(c.EntityIDs(), " ")))
	}
	for _, c := range part.Circles {
		fmt.Fprintf(&s, `<circle class="hole" cx="%s" cy="%s" r="%s"><title>%s</title></circle>`,
			num(c.Center.X), num(c.Center.Y), num(c.Radius), html.EscapeString(strings.Join(c.EntityIDs, " ")))
	}
	for _, n := range part.NonCut {
		if n.Kind == geometry.NonCutBend && n.Line != nil {
			fmt.Fprintf(&s, `<line class="bend" x1="%s" y1="%s" x2="%s" y2="%s"><title>%s</title></line>`,
				num(n.Line[0].X), num(n.Line[0].Y), num(n.Line[1].X), num(n.Line[1].Y), html.EscapeString(n.EntityID))
		}
	}
	for _, f := range findings {
		if f.Location == nil {
			continue
		}
		title := html.EscapeString(string(f.Code) + ": " + f.Message)
		if f.Span != nil {
			fmt.Fprintf(&s, `<line class="span" x1="%s" y1="%s" x2="%s" y2="%s"/>`,
				f.Span[0].X.String(), f.Span[0].Y.String(), f.Span[1].X.String(), f.Span[1].Y.String())
		}
		class := "finding warning"
		if f.Severity == constants.SeverityError {
			class = "finding error"
		}
		fmt.Fprintf(&s, `<circle class="%s" cx="%s" cy="%s" r="%s"><title>%s</title></circle>`,
			class, f.Location.X.String(), f.Location.Y.String(), num(marker), title)
	}
	s.WriteString(`</g></svg>`)
	s.WriteString("\n")
	return []byte(s.String())
}

// pathData writes a contour as SVG path data, arcs as arcs.
func pathData(c geometry.Contour) string {
	var d strings.Builder
	for i, seg := range c.Segments {
		if i == 0 {
			fmt.Fprintf(&d, "M%s %s", num(seg.Start.X), num(seg.Start.Y))
		}
		if !seg.IsArc() {
			fmt.Fprintf(&d, "L%s %s", num(seg.End.X), num(seg.End.Y))
			continue
		}
		large, sweep := 0, 0
		if math.Abs(seg.Bulge) > 1 {
			large = 1
		}
		if seg.Bulge > 0 {
			sweep = 1
		}
		r := seg.Radius()
		fmt.Fprintf(&d, "A%s %s 0 %d %d %s %s", num(r), num(r), large, sweep, num(seg.End.X), num(seg.End.Y))
	}
	if c.Closed {
		d.WriteString("Z")
	}
	return d.String()
}

// num formats a coordinate in millimeters to the nearest 0.1 µm.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

package design

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm/constants"
)

// svgStyle is the part of an SVG element's computed style the engine reads.
type svgStyle struct {
	stroke, fill string // normalized color (#rrggbb) or "none"
	hidden       bool   // display:none, visibility:hidden or opacity 0
}

// operation decides what a shape is for. Hidden or invisible shapes are construction geometry. Otherwise the operation set for the shape's color (its stroke, or its fill when it has none) applies; failing that, stroked shapes are cut and shapes only filled follow ops.Filled, by default area engraving (the convention of laser services that take SVG).
func (s svgStyle) operation(ops Operations) shapeOp {
	paint, def := s.stroke, constants.ShapeOperationCut
	if paint == "none" {
		paint, def = s.fill, ops.Filled
		if def == "" {
			def = constants.ShapeOperationEngrave
		}
	}
	if s.hidden || paint == "none" {
		return opConstruction
	}
	op, ok := ops.ByColor[paint]
	if !ok {
		op = def
	}
	return operationOf(op)
}

// inherit returns the style of a child: stroke, fill and visibility pass down, and a hidden ancestor hides everything below it.
func (s svgStyle) inherit(props map[string]string) svgStyle {
	out := s
	if v, ok := props["stroke"]; ok && v != "inherit" {
		out.stroke = normalizeColor(v)
	}
	if v, ok := props["fill"]; ok && v != "inherit" {
		out.fill = normalizeColor(v)
	}
	if props["display"] == "none" || props["visibility"] == "hidden" || props["visibility"] == "collapse" {
		out.hidden = true
	}
	for _, p := range []string{"opacity", "stroke-opacity", "fill-opacity"} {
		v, ok := props[p]
		if !ok {
			continue
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f <= 0 {
			switch p {
			case "opacity":
				out.hidden = true
			case "stroke-opacity":
				out.stroke = "none"
			default:
				out.fill = "none"
			}
		}
	}
	return out
}

// rootStyle is SVG's initial style: filled black, not stroked.
var rootStyle = svgStyle{stroke: "none", fill: "#000000"}

// cssRule is one declaration block of a <style> sheet, for one selector.
type cssRule struct {
	tag, class, id string
	specificity    int
	props          map[string]string
}

// parseCSS reads the simple style sheets SVG writers emit: rules whose selectors are a tag, .class, #id or tag.class, in comma lists. Anything else (combinators, attributes, at-rules) is ignored.
func parseCSS(sheet string) []cssRule {
	var out []cssRule
	sheet = stripComments(sheet)
	for {
		open := strings.IndexByte(sheet, '{')
		if open < 0 {
			return out
		}
		close := strings.IndexByte(sheet[open:], '}')
		if close < 0 {
			return out
		}
		selectors, body := sheet[:open], sheet[open+1:open+close]
		sheet = sheet[open+close+1:]
		props := parseDeclarations(body)
		for _, sel := range strings.Split(selectors, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" || strings.ContainsAny(sel, " >+~[:@*") {
				continue
			}
			var r cssRule
			if i := strings.IndexByte(sel, '#'); i >= 0 {
				r.tag, r.id, r.specificity = sel[:i], sel[i+1:], 100
			} else if i := strings.IndexByte(sel, '.'); i >= 0 {
				r.tag, r.class, r.specificity = sel[:i], sel[i+1:], 10
			} else {
				r.tag, r.specificity = sel, 1
			}
			if r.tag != "" && (r.class != "" || r.id != "") {
				r.specificity++
			}
			r.props = props
			out = append(out, r)
		}
	}
}

func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+2+j+2:]
	}
}

// parseDeclarations reads "name: value; ..." into lower-case names.
func parseDeclarations(s string) map[string]string {
	out := map[string]string{}
	for _, d := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "!important"))
		out[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	return out
}

// styleProps returns an element's own style properties: presentation attributes, overridden by matching style-sheet rules (sheet is sorted by specificity), overridden by its style attribute.
func styleProps(n *svgNode, sheet []cssRule) map[string]string {
	props := map[string]string{}
	for _, name := range []string{"stroke", "fill", "display", "visibility", "opacity", "stroke-opacity", "fill-opacity"} {
		if v, ok := n.attrs[name]; ok {
			props[name] = strings.TrimSpace(v)
		}
	}
	classes := strings.Fields(n.attrs["class"])
	for _, r := range sheet {
		if r.tag != "" && r.tag != n.name || r.id != "" && r.id != n.attrs["id"] {
			continue
		}
		if r.class != "" && !contains(classes, r.class) {
			continue
		}
		for k, v := range r.props {
			props[k] = v
		}
	}
	for k, v := range parseDeclarations(n.attrs["style"]) {
		props[k] = v
	}
	return props
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// namedColors covers the color keywords seen in laser-cutting files; any other keyword is kept as written, which still counts as a color.
var namedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "lime": "#00ff00", "green": "#008000",
	"blue": "#0000ff", "yellow": "#ffff00", "cyan": "#00ffff", "aqua": "#00ffff", "magenta": "#ff00ff",
	"fuchsia": "#ff00ff", "gray": "#808080", "grey": "#808080", "lightgray": "#d3d3d3",
	"lightgrey": "#d3d3d3", "darkgray": "#a9a9a9", "silver": "#c0c0c0", "orange": "#ffa500",
	"purple": "#800080", "navy": "#000080", "maroon": "#800000", "olive": "#808000", "teal": "#008080",
}

// normalizeColor returns a paint as "#rrggbb", "none", or the keyword written, lower-cased. Paint servers (url(...)) count as a color.
func normalizeColor(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case v == "" || v == "none" || v == "transparent":
		return "none"
	case strings.HasPrefix(v, "#") && len(v) == 4:
		return "#" + string([]byte{v[1], v[1], v[2], v[2], v[3], v[3]})
	case strings.HasPrefix(v, "#") && len(v) >= 7:
		return v[:7]
	case strings.HasPrefix(v, "rgb(") || strings.HasPrefix(v, "rgba("):
		inner := strings.TrimSuffix(v[strings.IndexByte(v, '(')+1:], ")")
		parts := strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) >= 4 {
			if a, err := strconv.ParseFloat(strings.TrimSuffix(parts[3], "%"), 64); err == nil && a <= 0 {
				return "none"
			}
		}
		if len(parts) < 3 {
			return v
		}
		var rgb [3]int
		for i := range 3 {
			p := strings.TrimSpace(parts[i])
			f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
			if err != nil {
				return v
			}
			if strings.HasSuffix(p, "%") {
				f *= 2.55
			}
			rgb[i] = int(math.Round(math.Max(0, math.Min(255, f))))
		}
		return fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
	}
	if c, ok := namedColors[v]; ok {
		return c
	}
	return v
}

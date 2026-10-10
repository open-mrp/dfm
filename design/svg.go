package design

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// Limits on what an SVG may contain or expand to, so a small file cannot make a check take minutes.
const (
	maxSVGNodes    = 200_000
	maxSVGDepth    = 256
	maxSVGUseDepth = 16
	maxSVGShapes   = 100_000
)

// svgNode is one element of an SVG document. Attributes are keyed by local name; those of the Inkscape and Sodipodi namespaces keep their prefix.
type svgNode struct {
	name     string
	attrs    map[string]string
	children []*svgNode
	text     string // character data, kept for <style>
	ordinal  int    // 1-based position among drawable elements
	// start and end are the byte offsets of the element in the file, from its start tag to the end of its end tag; tagEnd is the end of its start tag. A corrected file is written by editing these spans.
	start, tagEnd, end int64
}

// drawable lists the elements that produce geometry or are reported.
var drawable = map[string]bool{
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "image": true, "use": true,
}

// notRendered lists elements whose content is only drawn by reference, or never: definitions, clipping and masking, metadata.
var notRendered = map[string]bool{
	"defs": true, "symbol": true, "clipPath": true, "mask": true, "marker": true, "pattern": true,
	"metadata": true, "title": true, "desc": true, "style": true, "script": true,
	"foreignObject": true, "namedview": true, "linearGradient": true, "radialGradient": true, "filter": true,
}

// ParseSVG parses an SVG drawing into a normalized part.
//
// Stroked shapes are cut; shapes only filled are area engraving, and hidden or invisible ones construction geometry, both left out of the part (see svgStyle.operation). Groups whose Inkscape label or id names a bend or engraving layer are treated like DXF layers of that name. Text is reported as text_not_outlined and raster images as engraving.
//
// Sizes come from the root's width and height with their units and its viewBox. Sizes in pixels are read at the resolution of the program that wrote the file (72 per inch for Illustrator, 90 for Inkscape before 0.92, otherwise the standard 96), and the assumption is reported. units overrides all of it: one viewBox unit is then one of units.
func ParseSVG(data []byte, units constants.LengthUnit) (*geometry.Part, error) {
	return parseSVG(data, units, Operations{}, nil, geometry.BuildOptions{})
}

func parseSVG(data []byte, units constants.LengthUnit, ops Operations, adj []Adjustment, opts geometry.BuildOptions) (*geometry.Part, error) {
	root, err := readSVG(data)
	if err != nil {
		return nil, err
	}
	ids := map[string]*svgNode{}
	var sheet []cssRule
	var index func(n *svgNode)
	index = func(n *svgNode) {
		if id := n.attrs["id"]; id != "" {
			if _, dup := ids[id]; !dup {
				ids[id] = n
			}
		}
		if n.name == "style" {
			sheet = append(sheet, parseCSS(n.text)...)
		}
		for _, ch := range n.children {
			index(ch)
		}
	}
	index(root)
	// Rules apply in specificity order, ties in sheet order.
	slices.SortStableFunc(sheet, func(a, b cssRule) int { return a.specificity - b.specificity })

	place, drawn, err := svgViewport(root, data, units)
	if err != nil {
		return nil, err
	}
	r := &svgRenderer{
		top:   root,
		c:     converter{scale: 1, xf: place, zScale: 1},
		ids:   ids,
		sheet: sheet,
		ops:   ops,
	}
	if err := r.render(root, place, rootStyle, layerNone, "", 0); err != nil {
		return nil, err
	}
	c := &r.c
	if err := c.adjust(adj); err != nil {
		return nil, err
	}
	outside := offPage(root, place, c, ops.OutsidePage == constants.OutsidePageIgnore)
	if len(c.paths) == 0 && len(c.circles) == 0 {
		if len(c.nonCut) > 0 {
			return nil, fmt.Errorf("%w: no cut geometry: all %d shapes are engraving, construction or hidden (unless the capability says otherwise, only stroked shapes are cut)", ErrInvalid, len(c.nonCut))
		}
		return nil, fmt.Errorf("%w: no cut geometry found in the SVG", ErrInvalid)
	}
	if err := tooManySegments(c.paths, c.circles); err != nil {
		return nil, err
	}
	part := buildWithBendLines(c.paths, c.circles, opts, c.nonCut)
	part.Units = drawn.units
	part.UnitsAssumed = drawn.assumed
	part.ZeroLength, part.ZeroLengthAt = c.zeroLength, c.zeroAt
	part.Text = c.texts
	part.NonCut = append(c.nonCut, part.NonCut...)
	part.OutsidePage = outside
	return part, nil
}

// offPage finds the cut shapes that lie wholly outside the page (the viewBox), such as previews or notes drawn beside it, and, with ignore, leaves them out of c as ignored geometry.
func offPage(root *svgNode, place affine, c *converter, ignore bool) []geometry.OutsidePage {
	nums, err := numbers(root.attrs["viewBox"])
	if err != nil || len(nums) != 4 {
		return nil
	}
	page := geometry.EmptyBounds()
	for _, corner := range [][2]float64{{nums[0], nums[1]}, {nums[0] + nums[2], nums[1] + nums[3]}} {
		x, y := place.apply(corner[0], corner[1])
		page.Add(geometry.Point{X: x, Y: y})
	}
	// A shape is off the page only if every piece of it is.
	shapes := map[string]geometry.Bounds{}
	var order []string
	grow := func(id string, b geometry.Bounds) {
		cur, ok := shapes[id]
		if !ok {
			cur = geometry.EmptyBounds()
			order = append(order, id)
		}
		cur.Union(b)
		shapes[id] = cur
	}
	for _, p := range c.paths {
		for _, s := range p.Segments {
			grow(s.EntityID, s.Bounds())
		}
	}
	for _, k := range c.circles {
		grow(k.EntityIDs[0], k.Bounds())
	}
	off := map[string]bool{}
	var out []geometry.OutsidePage
	for _, id := range order {
		b := shapes[id]
		if b.MinX > page.MaxX || b.MaxX < page.MinX || b.MinY > page.MaxY || b.MaxY < page.MinY {
			off[id] = true
			out = append(out, geometry.OutsidePage{EntityID: id, Location: geometry.Point{X: b.MinX, Y: b.MinY}, Ignored: ignore})
		}
	}
	if !ignore || len(off) == 0 {
		return out
	}
	paths := c.paths[:0]
	for _, p := range c.paths {
		if !off[p.Segments[0].EntityID] {
			paths = append(paths, p)
		}
	}
	c.paths = paths
	circles := c.circles[:0]
	for _, k := range c.circles {
		if !off[k.EntityIDs[0]] {
			circles = append(circles, k)
		}
	}
	c.circles = circles
	for _, o := range out {
		loc := o.Location
		c.nonCut = append(c.nonCut, geometry.NonCut{EntityID: o.EntityID, Kind: geometry.NonCutIgnored, Position: &loc})
	}
	return out
}

// readSVG parses the XML into a tree, tolerating the entity references Illustrator declares in its DOCTYPE.
func readSVG(data []byte) (*svgNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	// A file declaring a single-byte encoding (Windows-1251, ISO-8859-x) is read byte for byte: geometry is ASCII in every one of them, and only text content would be misread.
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "utf-16", "utf-16le", "utf-16be", "utf-32", "shift_jis", "euc-jp", "gb2312", "gbk", "big5", "euc-kr":
			return nil, fmt.Errorf("encoding %q is not read", label)
		}
		return latin1Reader{in}, nil
	}
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var root *svgNode
	var stack []*svgNode
	nodes, ordinal := 0, 0
	for {
		at := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: not well-formed XML: %v", ErrInvalid, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if nodes++; nodes > maxSVGNodes {
				return nil, fmt.Errorf("%w: the SVG has more than %d elements", ErrUnsupportedContent, maxSVGNodes)
			}
			if len(stack) >= maxSVGDepth {
				return nil, fmt.Errorf("%w: the SVG nests elements more than %d deep", ErrUnsupportedContent, maxSVGDepth)
			}
			n := &svgNode{name: t.Name.Local, attrs: map[string]string{}, start: at, tagEnd: dec.InputOffset()}
			for _, a := range t.Attr {
				key := a.Name.Local
				switch {
				case strings.Contains(a.Name.Space, "inkscape"):
					key = "inkscape:" + key
				case strings.Contains(a.Name.Space, "sodipodi"):
					key = "sodipodi:" + key
				case a.Name.Space != "" && !strings.Contains(a.Name.Space, "xlink") && !strings.Contains(a.Name.Space, "svg"):
					if _, taken := n.attrs[key]; taken {
						continue
					}
				}
				n.attrs[key] = a.Value
			}
			if drawable[n.name] {
				ordinal++
				n.ordinal = ordinal
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("%w: more than one root element", ErrInvalid)
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack[len(stack)-1].end = dec.InputOffset()
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].name == "style" {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil || root.name != "svg" {
		return nil, fmt.Errorf("%w: not an SVG document (no <svg> root element)", ErrInvalid)
	}
	return root, nil
}

// svgUnits records how a drawing's sizes were read.
type svgUnits struct {
	units   constants.LengthUnit // mm or in when the drawing names them, else ""
	assumed bool                 // a pixel size had to be assumed
}

// mmPerUnit gives millimeters per CSS absolute unit.
var mmPerUnit = map[string]float64{"mm": 1, "cm": 10, "q": 0.25, "in": 25.4, "pt": 25.4 / 72, "pc": 25.4 / 6}

var (
	illustratorGenerator = regexp.MustCompile(`Generator:\s*Adobe Illustrator`)
	inkscapeVersion      = regexp.MustCompile(`^(\d+)\.(\d+)`)
)

// pixelMM returns millimeters per pixel for the program that wrote the file.
func pixelMM(root *svgNode, data []byte) float64 {
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	if illustratorGenerator.Match(head) {
		return 25.4 / 72
	}
	if m := inkscapeVersion.FindStringSubmatch(root.attrs["inkscape:version"]); m != nil {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		if major == 0 && minor < 92 {
			return 25.4 / 90
		}
	}
	return 25.4 / 96
}

// svgViewport returns the transform from the root's user units to millimeters, from its width, height and viewBox.
func svgViewport(root *svgNode, data []byte, override constants.LengthUnit) (affine, svgUnits, error) {
	var vb []float64
	if v, ok := root.attrs["viewBox"]; ok {
		nums, err := numbers(v)
		if err != nil || len(nums) != 4 || nums[2] <= 0 || nums[3] <= 0 {
			return affine{}, svgUnits{}, fmt.Errorf("%w: invalid viewBox %q", ErrInvalid, truncate(v))
		}
		vb = nums
	}
	if override != "" {
		if !override.IsValid() {
			return affine{}, svgUnits{}, fmt.Errorf("%w: unknown units override %q", ErrInvalid, override)
		}
		k := float64(override.Nanometers()) / 1e6
		place := scaling(k, k)
		if vb != nil {
			place = place.then(translate(-vb[0], -vb[1]))
		}
		return place, svgUnits{units: override}, nil
	}
	px := pixelMM(root, data)
	var drawn svgUnits
	// size returns the root's width or height in millimeters; ok is false when it is missing or relative.
	size := func(name string) (mm float64, ok bool, err error) {
		v := strings.TrimSpace(root.attrs[name])
		if v == "" || strings.HasSuffix(v, "%") {
			return 0, false, nil
		}
		num := strings.TrimRight(v, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
		unit := strings.ToLower(strings.TrimSpace(v[len(num):]))
		f, perr := strconv.ParseFloat(strings.TrimSpace(num), 64)
		if perr != nil || f <= 0 || math.IsInf(f, 0) {
			return 0, false, fmt.Errorf("%w: invalid %s %q", ErrInvalid, name, truncate(v))
		}
		switch unit {
		case "", "px":
			drawn.assumed = true
			return f * px, true, nil
		case "mm", "cm":
			drawn.units = constants.LengthUnitMillimeter
		case "in", "pt", "pc":
			drawn.units = constants.LengthUnitInch
		}
		k, known := mmPerUnit[unit]
		if !known {
			return 0, false, fmt.Errorf("%w: %s %q is in units the engine cannot convert", ErrUnsupportedContent, name, truncate(v))
		}
		return f * k, true, nil
	}
	w, okW, err := size("width")
	if err != nil {
		return affine{}, svgUnits{}, err
	}
	h, okH, err := size("height")
	if err != nil {
		return affine{}, svgUnits{}, err
	}
	if vb == nil {
		// Without a viewBox, user units are pixels.
		drawn.assumed = true
		return scaling(px, px), drawn, nil
	}
	if !okW {
		w, drawn.assumed = vb[2]*px, true
	}
	if !okH {
		h, drawn.assumed = vb[3]*px, true
	}
	sx, sy := w/vb[2], h/vb[3]
	tx, ty := 0.0, 0.0
	align := strings.Fields(root.attrs["preserveAspectRatio"])
	if len(align) == 0 || align[0] != "none" {
		a, slice := "xMidYMid", false
		if len(align) > 0 {
			a = align[0]
		}
		if len(align) > 1 {
			slice = align[1] == "slice"
		}
		s := math.Min(sx, sy)
		if slice {
			s = math.Max(sx, sy)
		}
		sx, sy = s, s
		switch {
		case strings.HasPrefix(a, "xMid"):
			tx = (w - float64(vb[2]*s)) / 2
		case strings.HasPrefix(a, "xMax"):
			tx = w - float64(vb[2]*s)
		}
		switch {
		case strings.HasSuffix(a, "YMid"):
			ty = (h - float64(vb[3]*s)) / 2
		case strings.HasSuffix(a, "YMax"):
			ty = h - float64(vb[3]*s)
		}
	}
	return translate(tx, ty).then(scaling(sx, sy)).then(translate(-vb[0], -vb[1])), drawn, nil
}

// parseTransform reads an SVG transform list.
func parseTransform(v string) (affine, error) {
	m := identity
	s := strings.TrimSpace(v)
	for s != "" {
		open := strings.IndexByte(s, '(')
		close := strings.IndexByte(s, ')')
		if open < 0 || close < open {
			return identity, fmt.Errorf("%w: invalid transform %q", ErrInvalid, truncate(v))
		}
		name := strings.TrimSpace(strings.Trim(s[:open], ", \t\n"))
		args, err := numbers(s[open+1 : close])
		if err != nil {
			return identity, fmt.Errorf("%w: invalid transform %q: %v", ErrInvalid, truncate(v), err)
		}
		var t affine
		switch {
		case name == "matrix" && len(args) == 6:
			t = affine{a: args[0], c: args[1], b: args[2], d: args[3], tx: args[4], ty: args[5]}
		case name == "translate" && (len(args) == 1 || len(args) == 2):
			args = append(args, 0)
			t = translate(args[0], args[1])
		case name == "scale" && (len(args) == 1 || len(args) == 2):
			if len(args) == 1 {
				args = append(args, args[0])
			}
			t = scaling(args[0], args[1])
		case name == "rotate" && (len(args) == 1 || len(args) == 3):
			t = rotation(args[0] * math.Pi / 180)
			if len(args) == 3 {
				t = translate(args[1], args[2]).then(t).then(translate(-args[1], -args[2]))
			}
		case name == "skewX" && len(args) == 1:
			t = affine{a: 1, b: math.Tan(args[0] * math.Pi / 180), d: 1}
		case name == "skewY" && len(args) == 1:
			t = affine{a: 1, c: math.Tan(args[0] * math.Pi / 180), d: 1}
		default:
			return identity, fmt.Errorf("%w: invalid transform %q", ErrInvalid, truncate(v))
		}
		m = m.then(t)
		s = strings.TrimSpace(s[close+1:])
	}
	return m, nil
}

// layerKind values carried down a group: none, or the non-cut kind a group named for bends or engraving gives everything in it.
type svgLayer struct {
	set  bool
	kind geometry.NonCutKind
}

var layerNone = svgLayer{}

// svgRenderer walks the document, converting shapes.
type svgRenderer struct {
	top    *svgNode // the document element
	c      converter
	ids    map[string]*svgNode
	sheet  []cssRule
	ops    Operations
	shapes int
}

func (r *svgRenderer) render(n *svgNode, ctm affine, parent svgStyle, layer svgLayer, prefix string, useDepth int) error {
	if notRendered[n.name] {
		return nil
	}
	style := parent.inherit(styleProps(n, r.sheet))
	if t, ok := n.attrs["transform"]; ok {
		m, err := parseTransform(t)
		if err != nil {
			return err
		}
		ctm = ctm.then(m)
	}
	switch n.name {
	case "svg", "g", "a", "switch":
		if n.name == "svg" && n != r.top {
			// A nested <svg> is placed at its x and y.
			x, _ := attrFloat(n, "x")
			y, _ := attrFloat(n, "y")
			ctm = ctm.then(translate(x, y))
		}
		if n.name == "g" && !layer.set {
			for _, name := range []string{n.attrs["inkscape:label"], n.attrs["id"]} {
				if kind, ok := layerKind(name); ok {
					layer = svgLayer{set: true, kind: kind}
					break
				}
			}
		}
		for _, ch := range n.children {
			if err := r.render(ch, ctm, style, layer, prefix, useDepth); err != nil {
				return err
			}
		}
		return nil
	case "use":
		return r.use(n, ctm, style, layer, prefix, useDepth)
	}
	if !drawable[n.name] {
		return nil
	}
	if r.shapes++; r.shapes > maxSVGShapes {
		return fmt.Errorf("%w: the SVG draws more than %d shapes", ErrUnsupportedContent, maxSVGShapes)
	}
	id := prefix + svgID(n)
	r.c.xf = ctm
	switch n.name {
	case "text":
		t := geometry.Text{EntityID: id}
		if x, ok := firstNumber(n.attrs["x"]); ok {
			y, _ := firstNumber(n.attrs["y"])
			p := r.c.wpt(x, y)
			t.Position = &p
		}
		r.c.texts = append(r.c.texts, t)
		return nil
	case "image":
		x, _ := attrFloat(n, "x")
		y, _ := attrFloat(n, "y")
		p := r.c.wpt(x, y)
		r.c.nonCut = append(r.c.nonCut, geometry.NonCut{EntityID: id, Kind: geometry.NonCutEngrave, Position: &p})
		return nil
	}
	op := style.operation(r.ops)
	if op == opCut && !layer.set {
		return r.shape(&r.c, n, id)
	}
	// Convert it on its own, so it is still validated and can be located, then leave it out of the part.
	scratch := converter{scale: 1, xf: ctm, zScale: 1}
	if err := r.shape(&scratch, n, id); err != nil {
		return err
	}
	kind := geometry.NonCutIgnored
	switch {
	case op == opEngrave:
		kind = geometry.NonCutEngrave
	case op == opCut && layer.set:
		kind = layer.kind
	}
	if pos := scratch.first(); pos != nil || len(scratch.zeroLength) == 0 {
		nc := geometry.NonCut{EntityID: id, Kind: kind, Position: pos}
		if kind == geometry.NonCutBend && len(scratch.paths) == 1 {
			if a, b, ok := geometry.StraightEnds(scratch.paths[0]); ok {
				nc.Line = &[2]geometry.Point{a, b}
			}
		}
		r.c.nonCut = append(r.c.nonCut, nc)
	}
	return nil
}

// use draws the element a <use> refers to, placed at its x and y, with IDs prefixed by the use's own.
func (r *svgRenderer) use(n *svgNode, ctm affine, style svgStyle, layer svgLayer, prefix string, depth int) error {
	href := n.attrs["href"]
	if !strings.HasPrefix(href, "#") {
		return nil // a reference to another file draws nothing here
	}
	target, ok := r.ids[href[1:]]
	if !ok {
		return fmt.Errorf("%w: <use> %s refers to undefined element %q", ErrInvalid, svgID(n), truncate(href))
	}
	if depth >= maxSVGUseDepth {
		return fmt.Errorf("%w: <use> %s nests references more than %d deep (circular reference?)", ErrInvalid, svgID(n), maxSVGUseDepth)
	}
	x, _ := attrFloat(n, "x")
	y, _ := attrFloat(n, "y")
	ctm = ctm.then(translate(x, y))
	prefix += svgID(n) + "/"
	if target.name == "symbol" {
		for _, ch := range target.children {
			if err := r.render(ch, ctm, style.inherit(styleProps(target, r.sheet)), layer, prefix, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return r.render(target, ctm, style, layer, prefix, depth+1)
}

// svgID is an element's id, or "#n" for the n-th drawable element.
func svgID(n *svgNode) string {
	if id := n.attrs["id"]; id != "" {
		return id
	}
	return "#" + strconv.Itoa(n.ordinal)
}

func attrFloat(n *svgNode, name string) (float64, bool) {
	return firstNumber(n.attrs[name])
}

// firstNumber reads a length attribute's number, ignoring a unit suffix; lengths in drawing content are in user units.
func firstNumber(v string) (float64, bool) {
	s := &pathScanner{s: strings.TrimRight(strings.TrimSpace(v), "px")}
	f, err := s.number()
	return f, err == nil
}

// shape converts one shape element into c.
func (r *svgRenderer) shape(c *converter, n *svgNode, id string) error {
	num := func(name string) float64 { f, _ := attrFloat(n, name); return f }
	switch n.name {
	case "path":
		ops, err := parsePathData(n.attrs["d"])
		if err != nil {
			return fmt.Errorf("%w: path %s: %v", ErrInvalid, id, err)
		}
		return c.svgPath(ops, id)
	case "line":
		return c.svgPath([]pathOp{{kind: 'M', to: [2]float64{num("x1"), num("y1")}}, {kind: 'L', to: [2]float64{num("x2"), num("y2")}}}, id)
	case "polyline", "polygon":
		pts, err := numbers(n.attrs["points"])
		if err != nil {
			return fmt.Errorf("%w: %s %s: %v", ErrInvalid, n.name, id, err)
		}
		var ops []pathOp
		for i := 0; i+1 < len(pts); i += 2 {
			kind := byte('L')
			if i == 0 {
				kind = 'M'
			}
			ops = append(ops, pathOp{kind: kind, to: [2]float64{pts[i], pts[i+1]}})
		}
		if n.name == "polygon" && len(ops) > 0 {
			ops = append(ops, pathOp{kind: 'Z', to: ops[0].to})
		}
		return c.svgPath(ops, id)
	case "rect":
		x, y, w, h := num("x"), num("y"), num("width"), num("height")
		if w <= 0 || h <= 0 {
			return nil // not rendered
		}
		rx, okX := attrFloat(n, "rx")
		ry, okY := attrFloat(n, "ry")
		switch {
		case okX && !okY:
			ry = rx
		case okY && !okX:
			rx = ry
		}
		rx, ry = math.Min(math.Max(rx, 0), w/2), math.Min(math.Max(ry, 0), h/2)
		if rx == 0 || ry == 0 {
			return c.svgPath([]pathOp{
				{kind: 'M', to: [2]float64{x, y}}, {kind: 'L', to: [2]float64{x + w, y}},
				{kind: 'L', to: [2]float64{x + w, y + h}}, {kind: 'L', to: [2]float64{x, y + h}},
				{kind: 'Z', to: [2]float64{x, y}},
			}, id)
		}
		arc := func(tx, ty float64) pathOp {
			return pathOp{kind: 'A', rx: rx, ry: ry, sweep: true, to: [2]float64{tx, ty}}
		}
		// Sides vanish when the corners take the whole width or height (a slot shape); leave them out rather than draw them at no length.
		ops := []pathOp{{kind: 'M', to: [2]float64{x + rx, y}}}
		side := func(tx, ty float64) {
			if last := ops[len(ops)-1].to; last != [2]float64{tx, ty} {
				ops = append(ops, pathOp{kind: 'L', to: [2]float64{tx, ty}})
			}
		}
		side(x+w-rx, y)
		ops = append(ops, arc(x+w, y+ry))
		side(x+w, y+h-ry)
		ops = append(ops, arc(x+w-rx, y+h))
		side(x+rx, y+h)
		ops = append(ops, arc(x, y+h-ry))
		side(x, y+ry)
		ops = append(ops, arc(x+rx, y), pathOp{kind: 'Z', to: [2]float64{x + rx, y}})
		return c.svgPath(ops, id)
	case "circle", "ellipse":
		cx, cy := num("cx"), num("cy")
		rx, ry := num("r"), num("r")
		if n.name == "ellipse" {
			rx, ry = num("rx"), num("ry")
		}
		if rx <= 0 || ry <= 0 {
			return nil // not rendered
		}
		if k, ok := c.xf.similar(); ok && rx == ry {
			c.circles = append(c.circles, geometry.Circle{Center: c.wpt(cx, cy), Radius: rx * k, EntityIDs: []string{id}})
			return nil
		}
		return c.svgEllipse(cx, cy, rx, ry, 0, 0, 2*math.Pi, id, nil)
	}
	return nil
}

// svgPath converts path operations, one contour per subpath.
func (c *converter) svgPath(ops []pathOp, id string) error {
	var segs []geometry.Segment
	var cur, start [2]float64
	flush := func(closed bool) {
		if len(segs) > 0 {
			if closed && len(segs) > 1 && segs[len(segs)-1].End.Near(segs[0].Start) {
				segs[len(segs)-1].End = segs[0].Start
			}
			c.paths = append(c.paths, geometry.Contour{Segments: segs, Closed: closed && len(segs) > 1})
		}
		segs = nil
	}
	add := func(more []geometry.Segment) {
		if len(more) == 0 {
			return
		}
		if len(segs) > 0 {
			more[0].Start = segs[len(segs)-1].End
		}
		segs = append(segs, more...)
	}
	line := func(a, b [2]float64) {
		s, ok := c.seg(false, c.wpt(a[0], a[1]), c.wpt(b[0], b[1]), 0, id)
		if !ok {
			c.noteZeroLength(id, c.wpt(a[0], a[1]))
			return
		}
		add([]geometry.Segment{s})
	}
	for _, op := range ops {
		switch op.kind {
		case 'M':
			flush(false)
			cur, start = op.to, op.to
		case 'L':
			line(cur, op.to)
			cur = op.to
		case 'C':
			p0, c1, c2, p3 := cur, op.c1, op.c2, op.to
			curve := func(t float64) (geometry.Point, geometry.Point) {
				u := 1 - t
				// Each product is converted before it is added, so no CPU fuses them (see AGENTS.md).
				x := float64(u*u*u*p0[0]) + float64(3*u*u*t*c1[0]) + float64(3*u*t*t*c2[0]) + float64(t*t*t*p3[0])
				y := float64(u*u*u*p0[1]) + float64(3*u*u*t*c1[1]) + float64(3*u*t*t*c2[1]) + float64(t*t*t*p3[1])
				dx := float64(3*u*u*(c1[0]-p0[0])) + float64(6*u*t*(c2[0]-c1[0])) + float64(3*t*t*(p3[0]-c2[0]))
				dy := float64(3*u*u*(c1[1]-p0[1])) + float64(6*u*t*(c2[1]-c1[1])) + float64(3*t*t*(p3[1]-c2[1]))
				vx, vy := c.vec(dx, dy)
				return c.wpt(x, y), geometry.Point{X: vx, Y: vy}
			}
			if p0 == p3 && c1 == p0 && c2 == p0 {
				c.noteZeroLength(id, c.wpt(p0[0], p0[1]))
				break
			}
			fitted, err := geometry.FitArcs(curve, 0, 1, id)
			if err != nil {
				return fmt.Errorf("%w: path %s: %v", ErrUnsupportedContent, id, err)
			}
			add(fitted)
			cur = p3
		case 'A':
			if err := c.svgArc(cur, op, id, add, line); err != nil {
				return err
			}
			cur = op.to
		case 'Z':
			if cur != start {
				line(cur, start)
			}
			flush(true)
			cur = start
		}
	}
	flush(false)
	return nil
}

// svgArc converts an elliptical arc command from the current point, using the endpoint-to-center conversion of the SVG specification. A circular arc under a shape-keeping transform stays an exact arc; any other is fitted.
func (c *converter) svgArc(from [2]float64, op pathOp, id string, add func([]geometry.Segment), line func(a, b [2]float64)) error {
	to := op.to
	if from == to {
		return nil
	}
	rx, ry := op.rx, op.ry
	if rx == 0 || ry == 0 {
		line(from, to)
		return nil
	}
	phi := op.rotation * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	dx, dy := (from[0]-to[0])/2, (from[1]-to[1])/2
	x1, y1 := float64(cosP*dx)+float64(sinP*dy), float64(-sinP*dx)+float64(cosP*dy)
	if l := x1*x1/(rx*rx) + y1*y1/(ry*ry); l > 1 {
		rx, ry = rx*math.Sqrt(l), ry*math.Sqrt(l)
	}
	num := float64(rx*rx*ry*ry) - float64(rx*rx*y1*y1) - float64(ry*ry*x1*x1)
	den := float64(rx*rx*y1*y1) + float64(ry*ry*x1*x1)
	k := math.Sqrt(math.Max(0, num/den))
	if op.large == op.sweep {
		k = -k
	}
	cx1, cy1 := k*rx*y1/ry, -k*ry*x1/rx
	// Each product is converted before it is added, so no CPU fuses them (see AGENTS.md).
	cx := float64(cosP*cx1) - float64(sinP*cy1) + (from[0]+to[0])/2
	cy := float64(sinP*cx1) + float64(cosP*cy1) + (from[1]+to[1])/2
	angle := func(ux, uy float64) float64 { return math.Atan2(uy, ux) }
	t1 := angle((x1-cx1)/rx, (y1-cy1)/ry)
	t2 := angle((-x1-cx1)/rx, (-y1-cy1)/ry)
	dt := t2 - t1
	if op.sweep && dt < 0 {
		dt += 2 * math.Pi
	} else if !op.sweep && dt > 0 {
		dt -= 2 * math.Pi
	}
	// Radii a millionth apart (rounding in the file) are a circle: the arc then stays exact.
	if _, similar := c.xf.similar(); similar && math.Abs(rx-ry) <= 1e-6*math.Max(rx, ry) {
		s, ok := c.seg(false, c.wpt(from[0], from[1]), c.wpt(to[0], to[1]), math.Tan(dt/4), id)
		if !ok {
			c.noteZeroLength(id, c.wpt(from[0], from[1]))
			return nil
		}
		add([]geometry.Segment{s})
		return nil
	}
	return c.svgEllipse(cx, cy, rx, ry, phi, t1, t1+dt, id, add)
}

// svgEllipse converts an elliptical arc (a whole ellipse from 0 to 2π) in user units, fitted with arcs a quarter turn at a time. With add, the arc continues the current subpath; otherwise it is a contour of its own.
func (c *converter) svgEllipse(cx, cy, rx, ry, phi, t0, t1 float64, id string, add func([]geometry.Segment)) error {
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	// The curve runs forwards over u in [0, 1] whichever way the angle turns, so its derivative is the direction of travel.
	span := t1 - t0
	curve := func(u float64) (geometry.Point, geometry.Point) {
		t := t0 + span*u
		ct, st := math.Cos(t), math.Sin(t)
		// Each product is converted before it is added, so no CPU fuses them (see AGENTS.md).
		x := cx + float64(rx*ct*cosP) - float64(ry*st*sinP)
		y := cy + float64(rx*ct*sinP) + float64(ry*st*cosP)
		vx, vy := c.vec(span*(float64(-rx*st*cosP)-float64(ry*ct*sinP)), span*(float64(-rx*st*sinP)+float64(ry*ct*cosP)))
		return c.wpt(x, y), geometry.Point{X: vx, Y: vy}
	}
	n := max(1, int(math.Ceil(math.Abs(span)/(math.Pi/2)-1e-9)))
	var segs []geometry.Segment
	for i := range n {
		a, b := float64(i)/float64(n), float64(i+1)/float64(n)
		piece, err := geometry.FitArcs(curve, a, b, id)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrUnsupportedContent, id, err)
		}
		if len(segs) > 0 && len(piece) > 0 {
			piece[0].Start = segs[len(segs)-1].End
		}
		segs = append(segs, piece...)
	}
	if add != nil {
		add(segs)
		return nil
	}
	c.addCurve(segs)
	return nil
}

// latin1Reader reads each byte as the character with that code, so any single-byte encoding decodes to valid UTF-8 with its ASCII intact.
type latin1Reader struct{ r io.Reader }

func (l latin1Reader) Read(p []byte) (int, error) {
	buf := make([]byte, len(p)/2+1)
	n, err := l.r.Read(buf)
	out := p[:0]
	for _, b := range buf[:n] {
		out = utf8.AppendRune(out, rune(b))
	}
	return len(out), err
}

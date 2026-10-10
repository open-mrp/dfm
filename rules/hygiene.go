package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// ZeroLengthRule warns about entities with a piece shorter than geometry.EndpointTolerance (a line whose ends coincide, a sliver of arc, a polyline with repeated vertices). The pieces were dropped from the geometry, so they do not affect cutting, but they usually indicate a careless export.
type ZeroLengthRule struct{}

func (ZeroLengthRule) Code() constants.FindingCode { return constants.FindingCodeGeometryZeroLength }

func (ZeroLengthRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, id := range part.ZeroLength {
		f := Finding{
			Code:      constants.FindingCodeGeometryZeroLength,
			Severity:  constants.SeverityWarning,
			Message:   "Entity has a piece shorter than 0.001 mm; it is ignored",
			EntityIDs: []string{id},
		}
		if q := part.EntityPoint(id); q != nil {
			f.Location = at(*q)
		} else if q, ok := part.ZeroLengthAt[id]; ok {
			f.Location = at(q)
		}
		out = append(out, f)
	}
	return out
}

// TextRule warns about text entities. Text is not cut: lettering meant to be cut must be converted to outlines, but notes and labels are often left in a drawing on purpose, so this is not an error.
type TextRule struct{}

func (TextRule) Code() constants.FindingCode { return constants.FindingCodeTextNotOutlined }

func (TextRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, t := range part.Text {
		f := Finding{
			Code:      constants.FindingCodeTextNotOutlined,
			Severity:  constants.SeverityWarning,
			Message:   "Text is not cut; convert it to outlines if it should be",
			EntityIDs: []string{t.EntityID},
		}
		if t.Position != nil {
			f.Location = at(*t.Position)
		}
		out = append(out, f)
	}
	return out
}

// NonCutRule fails geometry drawn as construction or hidden lines (dashed
// line types, hidden or non-plotting layers): center lines and sketch axes
// look like cut paths, so whether to cut them is ambiguous. The geometry was left out of the part, so the other checks see the part without it.
type NonCutRule struct{}

func (NonCutRule) Code() constants.FindingCode { return constants.FindingCodeFileNoncutGeometry }

func (NonCutRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return nonCutFindings(part, geometry.NonCutConstruction, constants.FindingCodeFileNoncutGeometry, constants.SeverityError,
		"Geometry is drawn as a construction or hidden line; remove it or draw it as a cut line")
}

// BendLineRule warns about bend lines (geometry on a bend or fold layer). They are expected in a flat pattern, but they are not cut and bends are not checked.
type BendLineRule struct{}

func (BendLineRule) Code() constants.FindingCode { return constants.FindingCodeBendLineNotCut }

func (BendLineRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	if part.ThicknessMM > 0 {
		return nil // unfolded from a model: its bends are not drawn lines
	}
	return nonCutFindings(part, geometry.NonCutBend, constants.FindingCodeBendLineNotCut, constants.SeverityWarning,
		"Bend line is not cut; bending is checked as its own operation")
}

// SuspectedBendRule fails solid straight lines on a cut layer that run through the material from one cut to another: bend lines drawn as cuts. Cutting them would cut the part apart, so the drawing must say what they are.
type SuspectedBendRule struct{}

func (SuspectedBendRule) Code() constants.FindingCode { return constants.FindingCodeBendLineSuspected }

func (SuspectedBendRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return nonCutFindings(part, geometry.NonCutSuspectedBend, constants.FindingCodeBendLineSuspected, constants.SeverityError,
		"Line runs through the material like a bend line; put it on a BEND layer or draw it dashed")
}

// BorderRule warns about a sheet border drawn around the drawing on the cut layer: it was left out of the part, as services strip borders, so the parts inside it are checked as parts.
type BorderRule struct{}

func (BorderRule) Code() constants.FindingCode { return constants.FindingCodeBorderNotCut }

func (BorderRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	if len(part.Border) == 0 {
		return nil
	}
	f := Finding{
		Code:      constants.FindingCodeBorderNotCut,
		Severity:  constants.SeverityWarning,
		Message:   "Sheet border is not cut; it is left out of the part",
		EntityIDs: part.Border,
		Fix:       &Fix{Action: constants.FixActionRemove, EntityIDs: part.Border},
	}
	if part.BorderAt != nil {
		f.Location = at(*part.BorderAt)
	}
	return []Finding{f}
}

// AnnotationRule warns about geometry on a dimension, center-line, note, title or construction layer: it is left out of the part.
type AnnotationRule struct{}

func (AnnotationRule) Code() constants.FindingCode { return constants.FindingCodeAnnotationNotCut }

func (AnnotationRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return nonCutFindings(part, geometry.NonCutAnnotation, constants.FindingCodeAnnotationNotCut, constants.SeverityWarning,
		"Annotation is not cut")
}

// EngravingRule warns about geometry on an engraving or marking layer: it is not cut through and is not checked.
type EngravingRule struct{}

func (EngravingRule) Code() constants.FindingCode { return constants.FindingCodeEngravingNotCut }

func (EngravingRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return nonCutFindings(part, geometry.NonCutEngrave, constants.FindingCodeEngravingNotCut, constants.SeverityWarning,
		"Engraving is not cut through; it is not checked")
}

func nonCutFindings(part *geometry.Part, kind geometry.NonCutKind, code constants.FindingCode, sev constants.Severity, msg string) []Finding {
	var out []Finding
	for _, n := range part.NonCut {
		if n.Kind != kind {
			continue
		}
		f := Finding{Code: code, Severity: sev, Message: msg, EntityIDs: []string{n.EntityID}}
		if n.Position != nil {
			f.Location = at(*n.Position)
		}
		switch kind {
		case geometry.NonCutSuspectedBend:
			f.Fix = &Fix{Action: constants.FixActionMarkBendLine, EntityIDs: append([]string{n.EntityID}, n.Copies...)}
		case geometry.NonCutConstruction:
			f.Fix = &Fix{Action: constants.FixActionRemove, EntityIDs: []string{n.EntityID}}
		}
		out = append(out, f)
	}
	return out
}

// DuplicateRule warns about paths drawn exactly on top of other geometry. The copy was dropped, so it is not cut twice, but it usually indicates a careless export. EntityIDs are the copies only, so deleting them all is always safe.
type DuplicateRule struct{}

func (DuplicateRule) Code() constants.FindingCode { return constants.FindingCodeGeometryDuplicate }

func (DuplicateRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, d := range part.Duplicates {
		f := Finding{
			Code:      constants.FindingCodeGeometryDuplicate,
			Severity:  constants.SeverityWarning,
			Message:   "Path is drawn more than once; the copy is ignored",
			EntityIDs: d.Copies,
			Location:  at(d.Location),
			Fix:       &Fix{Action: constants.FixActionRemove, EntityIDs: d.Copies},
		}
		// Removing an entity that also draws kept geometry (a path whose later piece retraces its own outline) would delete the kept part too: no fix then.
		for _, id := range d.Copies {
			if part.EntityPoint(id) != nil {
				f.Fix = nil
				f.Message = "Part of a path repeats geometry drawn already; the repeat is ignored"
				break
			}
		}
		if d.GapMM > geometry.EndpointTolerance {
			f.Message = "Line is drawn beside another, closer than the kerf; the copy is ignored"
			if g, err := measure.FromMM(d.GapMM); err == nil {
				f.Actual = dec(g)
			}
		}
		out = append(out, f)
	}
	return out
}

// OverlapRule fails cut paths drawn more than once along a positive length: the laser would cut the same line twice, adding heat and leaving the toolpath ambiguous. ActualMM is the total shared length.
type OverlapRule struct{}

func (OverlapRule) Code() constants.FindingCode {
	return constants.FindingCodeGeometryOverlappingSegments
}

func (OverlapRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	if part.ThicknessMM > 0 {
		return nil // a model's flat pattern: FlatOverlapRule
	}
	var out []Finding
	for _, o := range part.Overlaps {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryOverlappingSegments,
			Severity:  constants.SeverityError,
			Message:   "Cut path is drawn more than once",
			EntityIDs: o.EntityIDs,
			Actual:    mm(o.LengthMM),
			Location:  at(o.Location),
		})
	}
	return out
}

// SelfIntersectionRule fails a cut path that touches or crosses itself: it no longer bounds a single region, so what is part and what is scrap is ambiguous.
type SelfIntersectionRule struct{}

func (SelfIntersectionRule) Code() constants.FindingCode {
	return constants.FindingCodeGeometrySelfIntersection
}

func (SelfIntersectionRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return contactFindings(part, true)
}

// ContactRule fails two distinct cut paths that touch or cross. Touching cuts leave zero material between them, and crossing cuts no longer describe separate holes or edges.
type ContactRule struct{}

func (ContactRule) Code() constants.FindingCode {
	return constants.FindingCodeGeometryContoursTouchOrCross
}

func (ContactRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	return contactFindings(part, false)
}

func contactFindings(part *geometry.Part, self bool) []Finding {
	if part.ThicknessMM > 0 {
		return nil // a model's flat pattern: FlatOverlapRule
	}
	code, msg := constants.FindingCodeGeometryContoursTouchOrCross, "Cut paths touch or cross"
	if self {
		code, msg = constants.FindingCodeGeometrySelfIntersection, "Cut path touches or crosses itself"
	}
	var out []Finding
	for _, c := range part.Contacts {
		if c.SameContour != self {
			continue
		}
		out = append(out, Finding{
			Code:      code,
			Severity:  constants.SeverityError,
			Message:   msg,
			EntityIDs: c.EntityIDs,
			Location:  at(c.Location),
		})
	}
	return out
}

// NotFlatRule fails cut entities that are not at the drawing's height: above or below the plane most entities share, or sloping across it. The engine projects them flat, which is not what a 3D drawing means. Actual is how far off the plane the entity reaches.
type NotFlatRule struct{}

func (NotFlatRule) Code() constants.FindingCode { return constants.FindingCodeGeometryNotFlat }

func (NotFlatRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, o := range part.OffPlane {
		f := Finding{
			Code:      constants.FindingCodeGeometryNotFlat,
			Severity:  constants.SeverityError,
			Message:   "Entity is not at the drawing's height",
			EntityIDs: []string{o.EntityID},
			Actual:    mm(o.DistanceMM),
		}
		if q := part.EntityPoint(o.EntityID); q != nil {
			f.Location = at(*q)
		}
		out = append(out, f)
	}
	return out
}

// CornerLoopRule warns about small loops at corners of paths, which were removed before checking.
type CornerLoopRule struct{}

func (CornerLoopRule) Code() constants.FindingCode { return constants.FindingCodeGeometryCornerLoop }

func (CornerLoopRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, l := range part.CornerLoops {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryCornerLoop,
			Severity:  constants.SeverityWarning,
			Message:   "Path makes small loops at corners (corner reliefs); they are ignored",
			EntityIDs: l.EntityIDs,
			Actual:    mm(l.SizeMM),
			Location:  at(l.Location),
		})
	}
	return out
}

// FacetedCurveRule warns about curves drawn as many short straight segments.
type FacetedCurveRule struct{}

func (FacetedCurveRule) Code() constants.FindingCode {
	return constants.FindingCodeGeometryFacetedCurve
}

func (FacetedCurveRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, f := range part.FacetedCurves {
		n := measure.Ratio(int64(f.Segments) * 1e6).Decimal() // a whole number
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryFacetedCurve,
			Severity:  constants.SeverityWarning,
			Message:   "Curve is drawn as many short straight segments; it will be cut as a polygon",
			EntityIDs: f.EntityIDs,
			Actual:    &n,
			Unit:      constants.UnitSegment,
			Location:  at(f.Location),
		})
	}
	return out
}

// CommonLineRule reports cut lines two parts share: an error unless the capability accepts common-line cutting.
type CommonLineRule struct{}

func (CommonLineRule) Code() constants.FindingCode { return constants.FindingCodeGeometryCommonLine }

func (CommonLineRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	sev := constants.SeverityError
	if p.LaserCutting != nil && p.LaserCutting.CommonLine == constants.CommonLineAccepted {
		sev = constants.SeverityWarning
	}
	var out []Finding
	for _, l := range part.CommonLines {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryCommonLine,
			Severity:  sev,
			Message:   "Parts share a cut line",
			EntityIDs: l.EntityIDs,
			Actual:    mm(l.LengthMM),
			Location:  at(l.Location),
		})
	}
	return out
}

// OutsidePageRule warns about SVG shapes wholly outside the page.
type OutsidePageRule struct{}

func (OutsidePageRule) Code() constants.FindingCode { return constants.FindingCodeGeometryOutsidePage }

func (OutsidePageRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	// One occurrence for them all: they are usually one drawing beside the page, located at its corner nearest the origin.
	if len(part.OutsidePage) == 0 {
		return nil
	}
	f := Finding{
		Code:     constants.FindingCodeGeometryOutsidePage,
		Severity: constants.SeverityWarning,
		Message:  "Shapes lie wholly outside the page",
	}
	corner := part.OutsidePage[0].Location
	for _, o := range part.OutsidePage {
		f.EntityIDs = append(f.EntityIDs, o.EntityID)
		corner = geometry.Point{X: math.Min(corner.X, o.Location.X), Y: math.Min(corner.Y, o.Location.Y)}
	}
	f.Location = at(corner)
	return []Finding{f}
}

// OvercutRule warns about paths that run on past their start over their own first stretch: an overcut, which lasers use to make sure a cut closes. The repeat was trimmed and the path closed. Actual is its length.
type OvercutRule struct{}

func (OvercutRule) Code() constants.FindingCode { return constants.FindingCodeGeometryOvercut }

func (OvercutRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, o := range part.Overcuts {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryOvercut,
			Severity:  constants.SeverityWarning,
			Message:   "Path runs on past its start over its own first stretch; the repeat is ignored",
			EntityIDs: o.EntityIDs,
			Actual:    mm(o.LengthMM),
			Location:  at(o.Location),
		})
	}
	return out
}

// FlatOverlapRule fails a flat pattern unfolded from a model whose outline overlaps itself: flanges that lie over each other or the base once unfolded cannot be cut from one sheet. Where the drawing rules would report crossing, touching or retraced paths in a drawing, an unfolded model reports this instead.
type FlatOverlapRule struct{}

func (FlatOverlapRule) Code() constants.FindingCode { return constants.FindingCodeFlatPatternOverlaps }

func (FlatOverlapRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	if part.ThicknessMM == 0 {
		return nil
	}
	var out []Finding
	add := func(ids []string, q geometry.Point) {
		out = append(out, Finding{
			Code:      constants.FindingCodeFlatPatternOverlaps,
			Severity:  constants.SeverityError,
			Message:   "Unfolded, the part overlaps itself: it cannot be cut from one flat sheet",
			EntityIDs: ids,
			Location:  at(q),
		})
	}
	for _, c := range part.Contacts {
		add(c.EntityIDs, c.Location)
	}
	for _, o := range part.Overlaps {
		add(o.EntityIDs, o.Location)
	}
	return out
}

package dfm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/rules"
)

// CheckResult is the outcome of a completed check.
type CheckResult struct {
	Process constants.Process
	// EngineVersion is the engine that produced the result.
	EngineVersion string
	// Verdict is not_manufacturable with any error finding, needs_review with review findings and no errors (a person must decide), and manufacturable otherwise.
	Verdict constants.Verdict
	// ErrorCount, ReviewCount and WarningCount count Findings by severity.
	ErrorCount, ReviewCount, WarningCount int
	Findings                              []rules.Finding
	// Skipped lists checks that did not run because the capability does not set their limit, so a manufacturable result is never mistaken for a complete one.
	Skipped []rules.CodeInfo
	// Parts lists the parts of the drawing, one per outer boundary, each checked separately.
	Parts []PartResult
}

// PartResult describes one part of the drawing.
type PartResult struct {
	// Number is 1-based; findings about this part carry it.
	Number int
	// EntityIDs are the part's outer boundary (open paths for an unclosed part); empty when the drawing has none.
	EntityIDs []string
	// Width and Height are the part's bounding box in the drawing.
	Width, Height measure.Length
	// Verdict is the part's, from the findings about it. Findings about the drawing as a whole are not counted.
	Verdict constants.Verdict
	// Unclosed marks a part whose outline is open paths: only drawing findings are reported for it until the outline is closed.
	Unclosed bool
	// CutLength is the length cut (every path and circle, along arcs), Pierces how many times the beam pierces (one per path and circle), and Area the part's area (its outline less its cut-outs; 0 for an unclosed part): what a quote is priced from.
	CutLength measure.Length
	Pierces   int
	Area      measure.Area
	// ErrorCount, ReviewCount and WarningCount count the findings about this part.
	ErrorCount, ReviewCount, WarningCount int
	// Holes lists the part's round holes by drawn size and operation.
	Holes []HoleGroup
	// Bends lists the part's bend lines as bent, for a bending check; empty for other processes.
	Bends []Bend
	// CopyOf is the number of an earlier part this one is identical to, or 0: the same cut shape, moved and rotated but not mirrored, with the same findings. Placement maps that part onto this one.
	CopyOf    int
	Placement geometry.Placement
}

// markCopies sets CopyOf on every part that repeats an earlier one exactly: the same shape and the same findings, apart from where they are.
func (res *CheckResult) markCopies(parts []*geometry.Part) {
	if len(parts) < 2 {
		return
	}
	sigs := make([]string, len(parts))
	for _, f := range res.Findings {
		for _, n := range f.Parts {
			sigs[n-1] += findingSignature(f) + "\n"
		}
	}
	for i := range sigs {
		for _, g := range res.Parts[i].Holes {
			if g.Operation != "" {
				sigs[i] += fmt.Sprintf("hole|%s|%s|%d\n", g.Diameter, g.Operation, g.Count)
			}
		}
		for _, op := range parts[i].HoleOps {
			sigs[i] += "side|" + op.Code + "|" + op.Side + "\n"
		}
		for _, b := range res.Parts[i].Bends {
			sigs[i] += fmt.Sprintf("bend|%s|%s|%s|%s\n", b.Length, b.Angle, b.Direction, b.InsideRadius)
		}
		lines := strings.Split(sigs[i], "\n")
		slices.Sort(lines)
		sigs[i] = strings.Join(lines, "\n")
	}
	var firsts []int
	for i, p := range parts {
		if p.Unclosed {
			continue
		}
		for _, r := range firsts {
			if sigs[r] != sigs[i] {
				continue
			}
			if pl, ok := geometry.SameShape(parts[r], p); ok {
				res.Parts[i].CopyOf, res.Parts[i].Placement = r+1, pl
				break
			}
		}
		if res.Parts[i].CopyOf == 0 {
			firsts = append(firsts, i)
		}
	}
}

// findingSignature is what a finding says, leaving out where.
func findingSignature(f rules.Finding) string {
	s := fmt.Sprintf("%s|%s|%s|%d", f.Code, f.Severity, f.Unit, len(f.Parts))
	for _, d := range []*measure.Decimal{f.Actual, f.Required} {
		if d != nil {
			s += "|" + d.String()
		} else {
			s += "|-"
		}
	}
	if f.RequiredBasis != nil {
		s += "|" + string(*f.RequiredBasis)
	}
	return s
}

// add records a finding about the given part numbers (none for the whole drawing).
func (res *CheckResult) add(f rules.Finding, parts []int) {
	f.Parts = parts
	switch f.Severity {
	case constants.SeverityError:
		res.ErrorCount++
		for _, n := range parts {
			res.Parts[n-1].ErrorCount++
		}
	case constants.SeverityReview:
		res.ReviewCount++
		for _, n := range parts {
			res.Parts[n-1].ReviewCount++
		}
	case constants.SeverityWarning:
		res.WarningCount++
		for _, n := range parts {
			res.Parts[n-1].WarningCount++
		}
	}
	res.Verdict = verdict(res.ErrorCount, res.ReviewCount)
	for _, n := range parts {
		p := &res.Parts[n-1]
		p.Verdict = verdict(p.ErrorCount, p.ReviewCount)
	}
	res.Findings = append(res.Findings, f)
}

// verdict is the verdict of findings with these counts.
func verdict(errors, reviews int) constants.Verdict {
	switch {
	case errors > 0:
		return constants.VerdictNotManufacturable
	case reviews > 0:
		return constants.VerdictNeedsReview
	}
	return constants.VerdictManufacturable
}

// Manufacturable reports whether the verdict is manufacturable.
func (res CheckResult) Manufacturable() bool { return res.Verdict == constants.VerdictManufacturable }

// assignIDs gives each finding an ID derived from its content (code, severity, entities, parts, values and location), so the same finding gets the same ID whenever the same design is checked, including on a newer engine that reports it unchanged. Identical findings are numbered apart.
func assignIDs(findings []rules.Finding) {
	seen := map[string]int{}
	for i := range findings {
		f := &findings[i]
		h := sha256.New()
		fmt.Fprintf(h, "%s|%s|%s|%v", f.Code, f.Severity, strings.Join(f.EntityIDs, ","), f.Parts)
		for _, d := range []*measure.Decimal{f.Actual, f.Required} {
			if d != nil {
				fmt.Fprintf(h, "|%s", d)
			} else {
				fmt.Fprint(h, "|-")
			}
		}
		if f.Location != nil {
			fmt.Fprintf(h, "|%s,%s", f.Location.X, f.Location.Y)
		}
		id := "fnd_" + hex.EncodeToString(h.Sum(nil))[:16]
		seen[id]++
		if n := seen[id]; n > 1 {
			id = fmt.Sprintf("%s_%d", id, n)
		}
		f.ID = id
	}
}

// attribute returns the numbers of the parts a drawing-level finding is about, in order: the parts owning the entities it names; failing that, the parts its location lies inside; failing that, the parts whose edge it lies on. None means the drawing as a whole.
func attribute(f rules.Finding, parts []*geometry.Part) []int {
	if f.Code == constants.FindingCodeFileMultipleParts {
		return nil // about how the drawing is split, not any one part
	}
	var owners []int
	for k, p := range parts {
		for _, id := range f.EntityIDs {
			if p.Owns(id) {
				owners = append(owners, k+1)
				break
			}
		}
	}
	if len(owners) > 0 || f.Location == nil {
		return owners
	}
	q := geometry.Point{X: f.Location.X.MM(), Y: f.Location.Y.MM()}
	for _, onEdge := range []bool{false, true} {
		var covering []int
		for k, p := range parts {
			if p.Covers(q, onEdge) {
				covering = append(covering, k+1)
			}
		}
		if len(covering) > 0 {
			return covering
		}
	}
	return nil
}

func describePart(n int, p *geometry.Part) PartResult {
	out := PartResult{Number: n, EntityIDs: []string{}, Verdict: constants.VerdictManufacturable}
	if p.Outline != nil {
		out.EntityIDs = p.Outline
	}
	if !p.Bounds.IsEmpty() {
		// Bounds of real drawings are far inside the representable range.
		out.Width, _ = measure.FromMM(p.Bounds.MaxX - p.Bounds.MinX)
		out.Height, _ = measure.FromMM(p.Bounds.MaxY - p.Bounds.MinY)
	}
	out.Unclosed = p.Unclosed
	out.CutLength, _ = measure.FromMM(p.CutLengthMM())
	out.Pierces = p.Pierces()
	if !p.Unclosed {
		area := 0.0
		for i, l := range p.Loops {
			if l.Depth == 0 {
				area += p.NetAreaMM2(i)
			}
		}
		out.Area, _ = measure.AreaFromMM2(area)
	}
	return out
}

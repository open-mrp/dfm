package dfm

import (
	"math"
	"slices"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// diameterMatchMM is how close a drawn hole must be to a selector's diameter: diameters are reported to the nanometer, so a value copied from a result always matches.
const diameterMatchMM = 0.0005

// applyHoleOps resolves the assignments against the drawing's round holes and returns the drawing with the selected holes resized, its HoleOps set, and its misses and conflicts recorded. parts numbers the holes for selectors limited to one part.
func applyHoleOps(part *geometry.Part, parts []*geometry.Part, as []capability.HoleAssignment, profile *capability.Profile) *geometry.Part {
	holes := part.RoundHoles()
	chosen := make([][]int, len(holes)) // assignments selecting each hole
	partOf := func(h geometry.Circle) int {
		for k, p := range parts {
			if len(h.EntityIDs) > 0 && p.Owns(h.EntityIDs[0]) {
				return k + 1
			}
		}
		return 0
	}
	var misses []geometry.HoleOpMiss
	for ai, a := range as {
		found := map[string]bool{}
		hit := false
		for hi, h := range holes {
			if a.Part > 0 && partOf(h) != a.Part {
				continue
			}
			match := false
			if a.Diameter > 0 {
				match = math.Abs(2*h.Radius-a.Diameter.MM()) <= diameterMatchMM
			} else {
				for _, id := range h.EntityIDs {
					if slices.Contains(a.EntityIDs, id) {
						match, found[id] = true, true
					}
				}
			}
			if match {
				chosen[hi] = append(chosen[hi], ai)
				hit = true
			}
		}
		switch {
		case a.Diameter > 0 && !hit:
			misses = append(misses, geometry.HoleOpMiss{Assignment: ai, DiameterMM: a.Diameter.MM()})
		case a.Diameter == 0:
			for _, id := range a.EntityIDs {
				if !found[id] {
					misses = append(misses, geometry.HoleOpMiss{Assignment: ai, EntityIDs: []string{id}, Location: part.EntityPoint(id)})
				}
			}
		}
	}
	ops := make([]*geometry.HoleOp, len(holes))
	var conflicts []geometry.HoleOpConflict
	for hi, h := range holes {
		switch len(chosen[hi]) {
		case 0:
			continue
		case 1:
		default:
			conflicts = append(conflicts, geometry.HoleOpConflict{EntityIDs: h.EntityIDs, Center: h.Center, Assignments: chosen[hi]})
			continue
		}
		a := as[chosen[hi][0]]
		op := profile.HoleOperations[a.Operation]
		ops[hi] = &geometry.HoleOp{
			EntityIDs: h.EntityIDs, Center: h.Center,
			DrawnRadius: h.Radius, Radius: op.HoleDiameter.MM() / 2,
			Code: a.Operation, Side: string(a.Side),
		}
		if op.Type == constants.HoleOperationTap {
			ops[hi].Side = ""
		}
	}
	out := part.WithHoleOps(holes, ops)
	out.HoleOpMisses, out.HoleOpConflicts = misses, conflicts
	return out
}

// HoleGroup is a part's round holes of one drawn size and operation.
type HoleGroup struct {
	// Diameter is the drawn diameter.
	Diameter  measure.Length
	Count     int
	EntityIDs []string
	// Operation is the code assigned, or "".
	Operation string
	// Available lists the operations offered for the stock whose edge distance (error tier) every hole of the group meets.
	Available []string
}

// holeGroups lists a part's round holes by drawn diameter and operation.
func holeGroups(p *geometry.Part, profile *capability.Profile) []HoleGroup {
	assigned := map[string]geometry.HoleOp{}
	for _, op := range p.HoleOps {
		assigned[op.EntityIDs[0]] = op
	}
	var codes []string
	for code, op := range profile.HoleOperations {
		if op.Available {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	var out []HoleGroup
	at := map[[2]string]int{}
	for _, h := range p.RoundHoles() {
		drawn, code := h.Radius, ""
		if len(h.EntityIDs) == 0 {
			continue
		}
		if op, ok := assigned[h.EntityIDs[0]]; ok {
			drawn, code = op.DrawnRadius, op.Code
		}
		d, err := measure.FromMM(2 * drawn)
		if err != nil {
			continue
		}
		var avail []string
		clearance := measure.Length(math.MaxInt64)
		if len(codes) > 0 {
			if c, err := measure.FromMM(p.Clearance(h.Center, h.EntityIDs)); err == nil {
				clearance = c
			}
		}
		for _, c := range codes {
			if meetsEdge(profile.HoleOperations[c].EdgeDistance, profile, clearance) {
				avail = append(avail, c)
			}
		}
		key := [2]string{d.String(), code}
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			out = append(out, HoleGroup{Diameter: d, Operation: code, Available: avail})
		} else {
			out[i].Available = slices.DeleteFunc(out[i].Available, func(c string) bool { return !slices.Contains(avail, c) })
		}
		out[i].Count++
		out[i].EntityIDs = append(out[i].EntityIDs, h.EntityIDs...)
	}
	return out
}

// meetsEdge reports whether clearance meets the error tier of an edge distance limit (nil: no limit).
func meetsEdge(lim *capability.Limit, p *capability.Profile, clearance measure.Length) bool {
	if lim == nil || lim.Error == nil {
		return true
	}
	kerf := measure.Length(0)
	if p.LaserCutting != nil {
		kerf = p.LaserCutting.KerfWidth
	}
	return !lim.Error.Required(p.Thickness, kerf).Below(clearance)
}

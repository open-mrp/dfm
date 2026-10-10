// Package capability defines what a manufacturing setup can produce, independent of any one job, and resolves it against a job's stock.
//
// A capability names its process and carries a block for that process (e.g. LaserCutting). Materials list the stock the setup accepts, each with its own per-process block. Wire decoding lives in package request.
package capability

import (
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// Capability describes one manufacturing setup.
type Capability struct {
	Process constants.Process
	// LaserCutting is set when Process is laser_cutting.
	LaserCutting *LaserCutting
	// Bending is set when the process is bending.
	Bending *Bending
	// Materials lists every material the setup can process. A job in any other material is reported as material_unsupported.
	Materials []Material
	// HoleOperations is the catalog of taps, countersinks and press-in fasteners the setup offers.
	HoleOperations []HoleOperation
}

// LaserCutting holds the laser-cutting limits of a setup.
type LaserCutting struct {
	// UsableWidth and UsableHeight bound the area the head can cut in one setup (not the nominal sheet or bed size). Parts may rotate 90°.
	UsableWidth  measure.Length
	UsableHeight measure.Length
	// KerfWidth is the default width of material removed by the beam.
	KerfWidth measure.Length
	// ContourClosureTolerance, when set, closes gaps between path ends up to this size before checking: a gap that small is lost in the kerf.
	ContourClosureTolerance *measure.Length
	// SupportSpacing is the distance between the bed's supports; 0 means parts are not checked for falling between them.
	SupportSpacing measure.Length
	// MultipleParts says how a drawing with several parts is treated; empty means MultiplePartsAccepted.
	MultipleParts constants.MultipleParts
	// ColorOperations says what shapes of each color in a vector drawing (SVG) are for; other colors follow the defaults.
	ColorOperations []ColorOperation
	// FilledShapes says what shapes filled but not stroked in a vector drawing are for; empty means ShapeOperationEngrave.
	FilledShapes constants.ShapeOperation
	// OutsidePage says what SVG shapes wholly outside the page are for; empty means OutsidePageCut.
	OutsidePage constants.OutsidePage
	// DrawingUnits lists the units drawings may be in; empty accepts any.
	DrawingUnits []constants.LengthUnit
	// CommonLine says whether parts may share a cut line; empty means CommonLineRejected.
	CommonLine constants.CommonLine
	Limits     LaserCuttingLimits
}

// ColorOperation maps one color, "#rrggbb" in lower case, to an operation.
type ColorOperation struct {
	Color     string
	Operation constants.ShapeOperation
}

// LaserCuttingLimits holds one Limit per checked feature dimension. Hole and slot limits are required; the others are optional and their rules are only checked when set.
type LaserCuttingLimits struct {
	HoleDiameter Limit
	SlotWidth    Limit
	Optional     OptionalLaserCuttingLimits
}

// OptionalLaserCuttingLimits are limits a capability may leave unset.
type OptionalLaserCuttingLimits struct {
	// WebWidth is the material between two cut-outs.
	WebWidth *Limit
	// FeatureSpacing is the gap between two cut-outs, as a spacing rule.
	FeatureSpacing *Limit
	// EdgeDistance is the material between a cut-out and the edge around it.
	EdgeDistance *Limit
	// PartWidth is the part's narrowest width over every direction.
	PartWidth *Limit
	// PartLength is the part's length at right angles to its narrowest width; with PartWidth it sets a minimum part size such as 6 x 10 mm.
	PartLength *Limit
	// PartArea is the finished part's area: its outline less its cut-outs. A part can pass PartWidth and PartLength and still be too small.
	PartArea *AreaLimit
	// SlenderPart sets a minimum part width that rises with the part's length (long narrow strips warp), by increasing MinLength.
	SlenderPart []SlenderTier
	// SlenderCutout sets a minimum cut-out width that rises with the cut-out's length (long narrow cut-outs warp the part), by increasing MinLength. Round holes are not checked.
	SlenderCutout []SlenderTier
	// CutoutWidth is the narrowest passage within a cut-out or edge notch (recognized slots and round holes have their own limits).
	CutoutWidth *Limit
	// FeatureWidth is the narrowest strip of material within one outline: a finger, tab or tongue.
	FeatureWidth *Limit
	// InternalCornerRadius is the smallest radius of an inside corner of the material; a sharp corner has radius 0.
	InternalCornerRadius *Limit
	// SlotFitClearance is the least a slot that receives the stock (one within 25% of the thickness wide) may be wider than the stock.
	SlotFitClearance *Limit
	// MaxRemovedAreaPercent warns when cut-outs remove at least this percentage of a part's area (millionths of a percent: 50% is 50,000,000).
	MaxRemovedAreaPercent *measure.Ratio
	// RemovedAreaWindow, when set, also applies MaxRemovedAreaPercent in every square window this size across each part, to find a dense patch on a large part.
	RemovedAreaWindow *measure.Length
	// MinTipAngle warns, in millionths of a degree, about corners that come to a sharper point, on the material or the cut-out side.
	MinTipAngle *measure.Ratio
}

// merge overrides each limit that o sets.
func (l OptionalLaserCuttingLimits) merge(o OptionalLaserCuttingLimits) OptionalLaserCuttingLimits {
	if o.WebWidth != nil {
		l.WebWidth = o.WebWidth
	}
	if o.FeatureSpacing != nil {
		l.FeatureSpacing = o.FeatureSpacing
	}
	if o.EdgeDistance != nil {
		l.EdgeDistance = o.EdgeDistance
	}
	if o.PartWidth != nil {
		l.PartWidth = o.PartWidth
	}
	if o.PartLength != nil {
		l.PartLength = o.PartLength
	}
	if o.PartArea != nil {
		l.PartArea = o.PartArea
	}
	if o.SlenderPart != nil {
		l.SlenderPart = o.SlenderPart
	}
	if o.SlenderCutout != nil {
		l.SlenderCutout = o.SlenderCutout
	}
	if o.RemovedAreaWindow != nil {
		l.RemovedAreaWindow = o.RemovedAreaWindow
	}
	if o.CutoutWidth != nil {
		l.CutoutWidth = o.CutoutWidth
	}
	if o.FeatureWidth != nil {
		l.FeatureWidth = o.FeatureWidth
	}
	if o.InternalCornerRadius != nil {
		l.InternalCornerRadius = o.InternalCornerRadius
	}
	if o.SlotFitClearance != nil {
		l.SlotFitClearance = o.SlotFitClearance
	}
	if o.MaxRemovedAreaPercent != nil {
		l.MaxRemovedAreaPercent = o.MaxRemovedAreaPercent
	}
	if o.MinTipAngle != nil {
		l.MinTipAngle = o.MinTipAngle
	}
	return l
}

// SlenderTier is a minimum width for parts at least MinLength long.
type SlenderTier struct {
	MinLength measure.Length
	Width     Limit
}

// Limit is a minimum dimension with two tiers. Below Error the feature cannot be made; at or above Error but below Warning it can be made with reduced quality. At least one tier is set.
type Limit struct {
	Error   *measure.Threshold
	Warning *measure.Threshold
}

// AreaLimit is a minimum area with two tiers, as Limit is for lengths. At least one tier is set.
type AreaLimit struct {
	Error   *measure.Area
	Warning *measure.Area
}

// Material is one material a setup can process.
type Material struct {
	// Code identifies the material in a job's stock, e.g. "aluminum_5052".
	Code string
	// LaserCutting is set when the capability's process is laser_cutting.
	LaserCutting *MaterialLaserCutting
	// Bending is set when the capability's process is bending.
	Bending *MaterialBending
}

// MaterialLaserCutting holds a material's laser-cutting limits.
type MaterialLaserCutting struct {
	// MaxThickness is the thickest stock the setup can cut.
	MaxThickness measure.Length
	// MinThickness is the thinnest stock the setup can cut; 0 means no lower bound.
	MinThickness measure.Length
	// Thicknesses lists the stock thicknesses the shop stocks, increasing; empty means any thickness in range.
	Thicknesses []measure.Length
	// KerfWidth overrides the capability's kerf width.
	KerfWidth *measure.Length
	// UsableArea overrides the capability's usable area: thick plate may only be cut on part of the bed.
	UsableArea *UsableArea
	// Overrides replace the capability's limits of the same name.
	Overrides LimitOverrides
	// ThicknessRanges, by increasing MaxThickness, override the material's kerf, usable area and limits for stock up to each thickness, like a published table: the first range the stock fits within applies.
	ThicknessRanges []ThicknessRange
}

// LimitOverrides are limits that replace the limit of the same name as a whole.
type LimitOverrides struct {
	HoleDiameter *Limit
	SlotWidth    *Limit
	Optional     OptionalLaserCuttingLimits
}

// ThicknessRange holds a material's kerf, usable area and limits for stock up to MaxThickness (inclusive).
type ThicknessRange struct {
	MaxThickness measure.Length
	KerfWidth    *measure.Length
	UsableArea   *UsableArea
	Overrides    LimitOverrides
}

// UsableArea is an area the head can cut in one setup.
type UsableArea struct {
	Width, Height measure.Length
}

// Stock is the material and size of one job's raw material.
type Stock struct {
	MaterialCode string
	// Form is the stock's shape; Thickness is set for sheet.
	Form      constants.StockForm
	Thickness measure.Length
}

// Profile is a capability resolved for one job's stock.
type Profile struct {
	Process      constants.Process
	MaterialCode string
	Thickness    measure.Length
	// Supported is false when the material is not in the capability; the capability's defaults are then used for the remaining checks.
	Supported    bool
	LaserCutting *LaserCuttingProfile
	Bending      *BendingProfile
	// HoleOperations is the catalog resolved for the stock, by code.
	HoleOperations map[string]*ResolvedHoleOperation
}

// LaserCuttingProfile holds concrete laser-cutting limits for one job.
type LaserCuttingProfile struct {
	UsableWidth  measure.Length
	UsableHeight measure.Length
	KerfWidth    measure.Length
	// ContourClosureTolerance is zero when gaps are not closed.
	ContourClosureTolerance measure.Length
	// SupportSpacing is zero when parts are not checked for falling.
	SupportSpacing measure.Length
	// MultipleParts is never empty.
	MultipleParts constants.MultipleParts
	// ColorOperations maps "#rrggbb" to an operation; FilledShapes is never empty.
	ColorOperations map[string]constants.ShapeOperation
	FilledShapes    constants.ShapeOperation
	// OutsidePage and CommonLine are never empty.
	OutsidePage  constants.OutsidePage
	CommonLine   constants.CommonLine
	DrawingUnits []constants.LengthUnit
	// MaxThickness is zero when the material is not supported.
	MaxThickness measure.Length
	MinThickness measure.Length
	Thicknesses  []measure.Length
	HoleDiameter Limit
	SlotWidth    Limit
	Optional     OptionalLaserCuttingLimits
}

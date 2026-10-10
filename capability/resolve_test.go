package capability

import (
	"reflect"
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// TestResolvePrecedence checks what replaces what: the material's kerf, area and limits over the capability's, and the first thickness range the stock fits over the material's; an unknown material keeps the defaults and is unsupported.
func TestResolvePrecedence(t *testing.T) {
	c := laser()
	c.LaserCutting.Limits.Optional.WebWidth, c.LaserCutting.Limits.Optional.EdgeDistance = limit(1), limit(1)
	kerf3, kerf6 := mm(0.3), mm(0.6)
	m := c.Materials[0].LaserCutting
	m.KerfWidth, m.UsableArea = &kerf3, &UsableArea{Width: mm(1000), Height: mm(2000)}
	m.Overrides = LimitOverrides{HoleDiameter: limit(2), Optional: OptionalLaserCuttingLimits{WebWidth: limit(2)}}
	m.ThicknessRanges = []ThicknessRange{
		{MaxThickness: mm(3), Overrides: LimitOverrides{Optional: OptionalLaserCuttingLimits{WebWidth: limit(3)}}},
		{MaxThickness: mm(12), KerfWidth: &kerf6, UsableArea: &UsableArea{Width: mm(800), Height: mm(800)}, Overrides: LimitOverrides{SlotWidth: limit(4)}},
	}
	thin, err := Resolve(c, Stock{MaterialCode: "mild_steel", Thickness: mm(1.5)})
	if err != nil {
		t.Fatal(err)
	}
	lc := thin.LaserCutting
	if !thin.Supported || lc.KerfWidth != kerf3 || lc.UsableWidth != mm(1000) || lc.HoleDiameter.Error.Min != mm(2) || lc.SlotWidth.Error.Min != mm(1) ||
		lc.Optional.WebWidth.Error.Min != mm(3) || lc.Optional.EdgeDistance.Error.Min != mm(1) || lc.MaxThickness != mm(20) {
		t.Errorf("1.5 mm: %+v", lc)
	}
	thick, _ := Resolve(c, Stock{MaterialCode: "mild_steel", Thickness: mm(10)})
	lc = thick.LaserCutting
	if lc.KerfWidth != kerf6 || lc.UsableWidth != mm(800) || lc.SlotWidth.Error.Min != mm(4) || lc.Optional.WebWidth.Error.Min != mm(2) {
		t.Errorf("10 mm: %+v", lc)
	}
	beyond, _ := Resolve(c, Stock{MaterialCode: "mild_steel", Thickness: mm(15)})
	if lc = beyond.LaserCutting; lc.KerfWidth != kerf3 || lc.SlotWidth.Error.Min != mm(1) || lc.Optional.WebWidth.Error.Min != mm(2) {
		t.Errorf("15 mm, past every range: %+v", lc)
	}
	other, _ := Resolve(c, Stock{MaterialCode: "brass", Thickness: mm(1.5)})
	if lc = other.LaserCutting; other.Supported || lc.KerfWidth != mm(0.15) || lc.MaxThickness != 0 || lc.HoleDiameter.Error.Min != mm(1.5) || lc.MultipleParts != constants.MultiplePartsAccepted {
		t.Errorf("unknown material: supported %v %+v", other.Supported, lc)
	}
	if _, err := Resolve(c, Stock{MaterialCode: "mild_steel"}); err == nil || err.Param != "stock.thickness" {
		t.Errorf("zero thickness: %v", err)
	}
}

// TestResolveHoleOperations checks availability at the thickness bounds and the material's edge-distance override.
func TestResolveHoleOperations(t *testing.T) {
	c := laser()
	six := mm(6)
	c.HoleOperations[0].EdgeDistance = limit(5)
	c.HoleOperations[0].Materials[0].MaxThickness = &six
	c.HoleOperations[0].Materials[0].EdgeDistance = limit(7)
	for _, tt := range []struct {
		thickness float64
		available bool
	}{{0.9, false}, {1, true}, {6, true}, {6.1, false}} {
		p, _ := Resolve(c, Stock{MaterialCode: "mild_steel", Thickness: mm(tt.thickness)})
		op := p.HoleOperations["tap_m4"]
		if op.Available != tt.available || op.EdgeDistance.Error.Min != mm(7) {
			t.Errorf("%v mm: available %v, edge %v", tt.thickness, op.Available, op.EdgeDistance.Error.Min)
		}
	}
	p, _ := Resolve(c, Stock{MaterialCode: "brass", Thickness: mm(2)})
	if op := p.HoleOperations["tap_m4"]; op.Available || op.EdgeDistance.Error.Min != mm(5) {
		t.Errorf("other material: available %v, edge %v", op.Available, op.EdgeDistance.Error.Min)
	}
}

// TestResolveBending checks the brake's defaults, the material's replacements and the resolved radius and die opening.
func TestResolveBending(t *testing.T) {
	c := brake()
	c.Bending.MaxAngle, c.Bending.Limits.FlangeLength = 135_000_000, &BendLimit{Limit: *limit(5)}
	c.Bending.Tooling = &BrakeTooling{PunchTipAngle: 88_000_000, PunchWidth: mm(12), PunchHeight: mm(120), DieWidth: mm(30), DieHeight: mm(60), DieOpening: measure.Threshold{ThicknessRatio: 8_000_000}}
	mb := c.Materials[0].Bending
	mb.ForceFactor, mb.KFactor, mb.MaxAngle = 1_500_000, 500_000, 120_000_000
	mb.InsideRadius = &measure.Threshold{Min: mm(2)}
	mb.Overrides.FlangeLength = &BendLimit{Limit: *limit(8), PlusInsideRadius: true}
	p, err := Resolve(c, Stock{MaterialCode: "mild_steel", Thickness: mm(1.5)})
	if err != nil {
		t.Fatal(err)
	}
	b := p.Bending
	if b.ForceFactor != 1_500_000 || b.KFactor != 500_000 || b.MaxAngle != 120_000_000 || b.InsideRadius.String() != "2" || b.Tooling.DieOpeningMM.String() != "12" ||
		b.FlangeLength.Error.Min != mm(8) || !b.FlangeLength.PlusInsideRadius || b.MaxThickness != mm(6) {
		t.Errorf("%+v radius %s die %s", b, b.InsideRadius, b.Tooling.DieOpeningMM)
	}
	other, _ := Resolve(c, Stock{MaterialCode: "brass", Thickness: mm(1.5)})
	if b := other.Bending; other.Supported || b.ForceFactor != 1_000_000 || b.KFactor != DefaultKFactor || b.InsideRadius.String() != "1.5" || b.FlangeLength.Error.Min != mm(5) {
		t.Errorf("unknown material: %+v", b)
	}
}

// TestMergeCoversEveryField sets every field of an override and checks merge carries each, so a limit added to the struct but not to merge fails here rather than being silently ignored.
func TestMergeCoversEveryField(t *testing.T) {
	check := func(name string, zero, full, merged reflect.Value) {
		for i := range full.NumField() {
			f := full.Type().Field(i)
			if !reflect.DeepEqual(merged.Field(i).Interface(), full.Field(i).Interface()) {
				t.Errorf("%s.merge does not carry %s", name, f.Name)
			}
			if !zero.Field(i).IsZero() {
				t.Errorf("%s: the zero value's %s is set", name, f.Name)
			}
		}
	}
	// fill sets every pointer field to a new value and every slice to one element.
	fill := func(v reflect.Value) {
		for i := range v.NumField() {
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Pointer:
				f.Set(reflect.New(f.Type().Elem()))
			case reflect.Slice:
				f.Set(reflect.MakeSlice(f.Type(), 1, 1))
			default:
				t.Fatalf("%s.%s is a %s: teach fill about it", v.Type().Name(), v.Type().Field(i).Name, f.Kind())
			}
		}
	}
	var o OptionalLaserCuttingLimits
	fill(reflect.ValueOf(&o).Elem())
	check("OptionalLaserCuttingLimits", reflect.ValueOf(OptionalLaserCuttingLimits{}), reflect.ValueOf(o), reflect.ValueOf(OptionalLaserCuttingLimits{}.merge(o)))
	var b BendingLimits
	fill(reflect.ValueOf(&b).Elem())
	check("BendingLimits", reflect.ValueOf(BendingLimits{}), reflect.ValueOf(b), reflect.ValueOf(BendingLimits{}.merge(b)))
}

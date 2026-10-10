package request

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/rules"
)

func example(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../examples/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func replace(t *testing.T, doc, old, new string) string {
	t.Helper()
	if !strings.Contains(doc, old) {
		t.Fatalf("mutation target %q not found", old)
	}
	return strings.Replace(doc, old, new, 1)
}

func TestDecodeCapability(t *testing.T) {
	valid := example(t, "laser.json")
	mut := func(old, new string) string { return replace(t, valid, old, new) }
	tests := []struct {
		name  string
		doc   string
		code  constants.ErrorCode // "" for success
		param string
		msg   string
	}{
		{"valid", valid, "", "", ""},
		{"not JSON", `{"schema_version":`, constants.ErrorCodeInvalidFormat, "capability", "not valid JSON"},
		{"trailing data", valid + "{}", constants.ErrorCodeInvalidFormat, "capability", "after the JSON"},
		{"unknown schema version", mut(`"3.0"`, `"2.0"`), constants.ErrorCodeParameterInvalid, "capability.schema_version", `"2.0" is not supported`},
		{"unsupported process", mut(`"process": "laser_cutting"`, `"process": "waterjet"`), constants.ErrorCodeParameterInvalid, "capability.process", "is not one of laser_cutting"},
		{"typo with suggestion", mut(`"kerf_width"`, `"kerf_widht"`), constants.ErrorCodeParameterUnknown, "capability.laser_cutting.kerf_widht", "did you mean capability.laser_cutting.kerf_width?"},
		{"unknown field", mut(`"schema_version"`, `"extra": "x", "schema_version"`), constants.ErrorCodeParameterUnknown, "capability.extra", "not a recognized parameter"},
		{"missing required", mut(`"kerf_width": { "value": "0.15", "unit": "mm" },`, ``), constants.ErrorCodeParameterMissing, "capability.laser_cutting.kerf_width", "is required"},
		{"null required", mut(`"kerf_width": { "value": "0.15", "unit": "mm" }`, `"kerf_width": null`), constants.ErrorCodeParameterMissing, "capability.laser_cutting.kerf_width", "is required"},
		{"null optional", mut(`"warning": { "thickness_ratio": "1" }
      },
      "slot_width"`, `"warning": null
      },
      "slot_width"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.hole_diameter.warning", "cannot be null"},
		{"number instead of string", mut(`"value": "1500"`, `"value": 1500`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.usable_area.width.value", "decimal values are sent as strings"},
		{"blank string", mut(`"value": "1500"`, `"value": ""`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.usable_area.width.value", "must not be blank"},
		{"bad unit", mut(`"value": "1500", "unit": "mm"`, `"value": "1500", "unit": "millimeters"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.usable_area.width.unit", "is not one of um, mm, cm, m, in, ft"},
		{"exponent", mut(`"value": "1500"`, `"value": "1.5e3"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.usable_area.width.value", "not a decimal number"},
		{"sub-nanometer", mut(`"value": "0.15"`, `"value": "0.1500001"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.kerf_width.value", "finer than 1 nanometer"},
		{"ratio too precise", mut(`"thickness_ratio": "1" }
      },
      "slot_width"`, `"thickness_ratio": "0.3333333" }
      },
      "slot_width"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.hole_diameter.warning.thickness_ratio", "more than 6 decimal places"},
		{"ratio as number", mut(`"thickness_ratio": "0.5"`, `"thickness_ratio": 0.5`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.hole_diameter.error.thickness_ratio", "must be a string"},
		{"empty threshold", mut(`"error": { "min": { "value": "1", "unit": "mm" }, "thickness_ratio": "0.5" }`, `"error": {}`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.slot_width.error", "needs at least one of"},
		{"empty limit", mut(`"slot_width": {
        "error": { "min": { "value": "1", "unit": "mm" }, "thickness_ratio": "0.5" },
        "warning": { "thickness_ratio": "1" }
      }`, `"slot_width": {}`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.slot_width", "needs an error and/or a warning"},
		{"zero usable width", mut(`"value": "1500"`, `"value": "0"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.usable_area.width", "must be greater than 0"},
		{"negative kerf", mut(`"value": "0.15"`, `"value": "-0.15"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.kerf_width", "must not be negative"},
		{"cap below min", mut(`"error": { "min": { "value": "0.5", "unit": "mm" }, "thickness_ratio": "0.5" }`, `"error": { "min": { "value": "0.5", "unit": "mm" }, "thickness_ratio": "0.5", "max": { "value": "0.4", "unit": "mm" } }`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.limits.web_width.error.max", "at least min"},
		{"zero closure tolerance", mut(`"contour_closure_tolerance": { "value": "0.05"`, `"contour_closure_tolerance": { "value": "0"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.contour_closure_tolerance", "greater than 0"},
		{"unknown multiple_parts mode", mut(`"multiple_parts": "accepted"`, `"multiple_parts": "sometimes"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.multiple_parts", "accepted_in_cutouts"},
		{"closure tolerance over 1 mm", mut(`"contour_closure_tolerance": { "value": "0.05"`, `"contour_closure_tolerance": { "value": "1.5"`), constants.ErrorCodeParameterInvalid, "capability.laser_cutting.contour_closure_tolerance", "at most 1 mm"},
		{"missing process block", edit(t, valid, func(m map[string]any) { delete(m, "laser_cutting") }), constants.ErrorCodeParameterMissing, "capability.laser_cutting", "is required"},
		{"no materials", edit(t, valid, func(m map[string]any) { m["materials"] = []any{} }), constants.ErrorCodeParameterInvalid, "capability.materials", "must not be empty"},
		{"materials not an array", edit(t, valid, func(m map[string]any) { m["materials"] = "steel" }), constants.ErrorCodeParameterInvalid, "capability.materials", "must be an array"},
		{"material without process block", edit(t, valid, func(m map[string]any) {
			delete(m["materials"].([]any)[0].(map[string]any), "laser_cutting")
		}), constants.ErrorCodeParameterMissing, "capability.materials[0].laser_cutting", "is required"},
		{"duplicate material", mut(`"code": "stainless_304"`, `"code": "mild_steel"`), constants.ErrorCodeParameterInvalid, "capability.materials[1].code", "more than once"},
		{"bad material code", mut(`"code": "mild_steel"`, `"code": "Mild Steel"`), constants.ErrorCodeParameterInvalid, "capability.materials[0].code", "lowercase letters"},
		{"null array item", mut(`"materials": [`, `"materials": [null,`), constants.ErrorCodeParameterInvalid, "capability.materials[0]", "cannot be null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeCapability([]byte(tt.doc))
			if tt.code == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted")
			}
			if err.Code != tt.code || err.Param != tt.param || !strings.Contains(err.Message, tt.msg) {
				t.Errorf("got %s param=%q %q; want %s param=%q containing %q", err.Code, err.Param, err.Message, tt.code, tt.param, tt.msg)
			}
			if err.Type != constants.ErrorTypeInvalidRequest {
				t.Errorf("type %s", err.Type)
			}
		})
	}
}

// edit applies f to the decoded JSON document and re-encodes it.
func edit(t *testing.T, doc string, f func(map[string]any)) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	f(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestCapabilityExactValues(t *testing.T) {
	c, err := DecodeCapability([]byte(example(t, "laser.json")))
	if err != nil {
		t.Fatal(err)
	}
	lc := c.LaserCutting
	if lc.UsableWidth != 1500*measure.Millimeter || lc.KerfWidth != 150_000 {
		t.Errorf("laser cutting %+v", lc)
	}
	h := lc.Limits.HoleDiameter
	if h.Error.Min != 1_500_000 || h.Error.ThicknessRatio != 500_000 || h.Warning.ThicknessRatio != 1_000_000 || h.Warning.Min != 0 {
		t.Errorf("hole limit %+v / %+v", *h.Error, *h.Warning)
	}
	if c.Materials[0].LaserCutting.MaxThickness != 19_050_000 {
		t.Errorf("max thickness %v", c.Materials[0].LaserCutting.MaxThickness)
	}
}

func TestInchCapability(t *testing.T) {
	doc := strings.ReplaceAll(example(t, "laser.json"), `"value": "1500", "unit": "mm"`, `"value": "59.055", "unit": "in"`)
	c, err := DecodeCapability([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if c.LaserCutting.UsableWidth != 1_499_997_000 { // 59.055 in = 1499.997 mm exactly
		t.Errorf("width %d nm", c.LaserCutting.UsableWidth)
	}
}

func TestResolve(t *testing.T) {
	c, err := DecodeCapability([]byte(example(t, "laser.json")))
	if err != nil {
		t.Fatal(err)
	}
	steel, rerr := capability.Resolve(c, capability.Stock{MaterialCode: "mild_steel", Thickness: 2 * measure.Millimeter})
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !steel.Supported || steel.LaserCutting.MaxThickness != 19_050_000 || steel.LaserCutting.KerfWidth != 150_000 {
		t.Errorf("steel %+v", steel.LaserCutting)
	}
	al, _ := capability.Resolve(c, capability.Stock{MaterialCode: "aluminum_5052", Thickness: 2 * measure.Millimeter})
	if r := al.LaserCutting.HoleDiameter.Error.Required(al.Thickness, al.LaserCutting.KerfWidth); r.Value.String() != "2" || al.LaserCutting.HoleDiameter.Warning != nil {
		t.Errorf("aluminum override not applied as a whole: %+v", al.LaserCutting.HoleDiameter)
	}
	if al.LaserCutting.SlotWidth.Error.Min != measure.Millimeter {
		t.Errorf("aluminum slot limit should be the default: %+v", al.LaserCutting.SlotWidth)
	}
	unknown, rerr := capability.Resolve(c, capability.Stock{MaterialCode: "titanium", Thickness: measure.Millimeter})
	if rerr != nil || unknown.Supported || unknown.LaserCutting.HoleDiameter.Error == nil {
		t.Errorf("unknown material %+v %v", unknown, rerr)
	}
	for _, s := range []struct {
		stock capability.Stock
		param string
	}{
		{capability.Stock{Thickness: measure.Millimeter}, "stock.material_code"},
		{capability.Stock{MaterialCode: "mild_steel"}, "stock.thickness"},
	} {
		if _, err := capability.Resolve(c, s.stock); err == nil || err.Param != s.param {
			t.Errorf("Resolve(%+v) = %v, want param %s", s.stock, err, s.param)
		}
	}
}

func TestDecodeCheckRequest(t *testing.T) {
	valid := example(t, "check-request.json")
	r, err := DecodeCheckRequest([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	s, serr := r.Stock.ToDomain()
	if serr != nil || s.Thickness != 1_500_000 || s.MaterialCode != "mild_steel" {
		t.Errorf("stock %+v %v", s, serr)
	}
	if d := r.Design.ToDomain(nil); d.Format != constants.DesignFormatDXF || d.Units != "" {
		t.Errorf("design %+v", d)
	}
	tests := []struct {
		old, new string
		code     constants.ErrorCode
		param    string
	}{
		{`"format": "dxf"`, `"format": "iges"`, constants.ErrorCodeParameterInvalid, "design.format"},
		{`"format": "dxf"`, `"format": "dxf", "units": "furlong"`, constants.ErrorCodeParameterInvalid, "design.units"},
		{`"format": "dxf"`, `"format": "dxf", "units": null`, constants.ErrorCodeParameterInvalid, "design.units"},
		{`"material_code": "mild_steel",`, ``, constants.ErrorCodeParameterMissing, "stock.material_code"},
		{`"thickness": { "value": "1.5", "unit": "mm" }`, `"thickness": { "value": 1.5, "unit": "mm" }`, constants.ErrorCodeParameterInvalid, "stock.thickness.value"},
		{`"format": "dxf"`, `"format": "dxf", "adjustments": [{ "action": "explode" }]`, constants.ErrorCodeParameterInvalid, "design.adjustments[0].action"},
		{`"format": "dxf"`, `"format": "dxf", "adjustments": [{ "action": "set_units", "units": "furlong" }]`, constants.ErrorCodeParameterInvalid, "design.adjustments[0].units"},
	}
	for _, tt := range tests {
		_, err := DecodeCheckRequest([]byte(replace(t, valid, tt.old, tt.new)))
		if err == nil || err.Code != tt.code || err.Param != tt.param {
			t.Errorf("%s → %s: got %v, want %s %s", tt.old, tt.new, err, tt.code, tt.param)
		}
	}
	withUnits, _ := DecodeCheckRequest([]byte(replace(t, valid, `"format": "dxf"`, `"format": "dxf", "units": "in"`)))
	if d := withUnits.Design.ToDomain(nil); d.Units != constants.LengthUnitInch {
		t.Errorf("units override %q", d.Units)
	}
	withAdj, err := DecodeCheckRequest([]byte(replace(t, valid, `"format": "dxf"`,
		`"format": "dxf", "adjustments": [{ "action": "mark_bend_line", "entity_ids": ["171", "175"] }, { "action": "set_units", "units": "in" }]`)))
	if err != nil {
		t.Fatal(err)
	}
	d := withAdj.Design.ToDomain(nil)
	if len(d.Adjustments) != 2 || d.Adjustments[0].Action != constants.FixActionMarkBendLine || len(d.Adjustments[0].EntityIDs) != 2 || d.Adjustments[1].Units != constants.LengthUnitInch {
		t.Errorf("adjustments %+v", d.Adjustments)
	}
}

func TestOptionalLimits(t *testing.T) {
	valid := example(t, "laser.json")
	c, err := DecodeCapability([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	o := c.LaserCutting.Limits.Optional
	if o.WebWidth.Error.Min != 500_000 || o.WebWidth.Error.ThicknessRatio != 500_000 || o.FeatureSpacing.Error != nil ||
		o.PartWidth.Error.Min != 2_540_000 || *o.MaxRemovedAreaPercent != 50_000_000 {
		t.Errorf("optional limits %+v", o)
	}
	// A material override replaces the whole limit, both tiers.
	withOverride := edit(t, valid, func(m map[string]any) {
		for _, mat := range m["materials"].([]any) {
			if mat := mat.(map[string]any); mat["code"] == "aluminum_5052" {
				mat["laser_cutting"].(map[string]any)["limits"].(map[string]any)["edge_distance"] =
					map[string]any{"error": map[string]any{"thickness_ratio": "2"}}
			}
		}
	})
	co, err := DecodeCapability([]byte(withOverride))
	if err != nil {
		t.Fatal(err)
	}
	al, _ := capability.Resolve(co, capability.Stock{MaterialCode: "aluminum_5052", Thickness: measure.Millimeter})
	if al.LaserCutting.Optional.EdgeDistance.Error.ThicknessRatio != 2_000_000 || al.LaserCutting.Optional.EdgeDistance.Warning != nil {
		t.Errorf("material edge_distance override not applied as a whole: %+v", al.LaserCutting.Optional.EdgeDistance)
	}
	if al.LaserCutting.Optional.WebWidth == nil {
		t.Error("unset material override dropped the default web_width")
	}

	// part_area is an area in any area unit; a material can override it.
	withArea := edit(t, valid, func(m map[string]any) {
		m["laser_cutting"].(map[string]any)["limits"].(map[string]any)["part_area"] =
			map[string]any{"error": map[string]any{"value": "1", "unit": "in2"}, "warning": map[string]any{"value": "1000", "unit": "mm2"}}
		for _, mat := range m["materials"].([]any) {
			if mat := mat.(map[string]any); mat["code"] == "aluminum_5052" {
				mat["laser_cutting"].(map[string]any)["limits"].(map[string]any)["part_area"] =
					map[string]any{"error": map[string]any{"value": "2", "unit": "cm2"}}
			}
		}
	})
	ca, err := DecodeCapability([]byte(withArea))
	if err != nil {
		t.Fatal(err)
	}
	if a := ca.LaserCutting.Limits.Optional.PartArea; a == nil || *a.Error != 645_160_000 || *a.Warning != 1_000_000_000 {
		t.Errorf("part_area: %+v", a)
	}
	al, _ = capability.Resolve(ca, capability.Stock{MaterialCode: "aluminum_5052", Thickness: measure.Millimeter})
	if a := al.LaserCutting.Optional.PartArea; *a.Error != 200_000_000 || a.Warning != nil {
		t.Errorf("material part_area override not applied as a whole: %+v", a)
	}
	for _, tt := range []struct{ area, param string }{
		{`{ "error": { "value": "0", "unit": "mm2" } }`, ".error"},
		{`{ "error": { "value": "1", "unit": "mm" } }`, ".error.unit"},
		{`{ "error": { "value": "0.0000001", "unit": "mm2" } }`, ".error.value"},
		{`{}`, ""},
	} {
		doc := replace(t, valid, `"max_removed_area_percent": "50"`, `"part_area": `+tt.area+`, "max_removed_area_percent": "0.5"`)
		if _, err := DecodeCapability([]byte(doc)); err == nil || err.Param != "capability.laser_cutting.limits.part_area"+tt.param {
			t.Errorf("part_area %s: %v", tt.area, err)
		}
	}

	without := edit(t, valid, func(m map[string]any) {
		lim := m["laser_cutting"].(map[string]any)["limits"].(map[string]any)
		for _, k := range []string{"web_width", "feature_spacing", "edge_distance", "part_width", "max_removed_area_percent"} {
			delete(lim, k)
		}
	})
	if c, err := DecodeCapability([]byte(without)); err != nil || c.LaserCutting.Limits.Optional.WebWidth != nil {
		t.Errorf("capability without optional limits: %+v %v", c.LaserCutting.Limits.Optional, err)
	}

	for _, v := range []string{"0", "150", "-10"} {
		doc := replace(t, valid, `"max_removed_area_percent": "50"`, `"max_removed_area_percent": "`+v+`"`)
		if _, err := DecodeCapability([]byte(doc)); err == nil || err.Param != "capability.laser_cutting.limits.max_removed_area_percent" {
			t.Errorf("fraction %s: %v", v, err)
		}
	}
	for v, param := range map[string]string{"0": "", "180": "", "-5": "", "30.1234567": ".value"} {
		doc := replace(t, valid, `"min_tip_angle": { "value": "30", "unit": "deg" }`, `"min_tip_angle": { "value": "`+v+`", "unit": "deg" }`)
		if _, err := DecodeCapability([]byte(doc)); err == nil || err.Param != "capability.laser_cutting.limits.min_tip_angle"+param {
			t.Errorf("tip angle %s: %v", v, err)
		}
	}
	rad := replace(t, valid, `"min_tip_angle": { "value": "30", "unit": "deg" }`, `"min_tip_angle": { "value": "30", "unit": "rad" }`)
	if _, err := DecodeCapability([]byte(rad)); err == nil || err.Param != "capability.laser_cutting.limits.min_tip_angle.unit" {
		t.Errorf("unknown angle unit: %v", err)
	}
	empty := replace(t, valid, `"feature_spacing": {
        "warning": { "thickness_ratio": "2" }
      }`, `"feature_spacing": {}`)
	if _, err := DecodeCapability([]byte(empty)); err == nil || err.Param != "capability.laser_cutting.limits.feature_spacing" {
		t.Errorf("empty feature_spacing: %v", err)
	}
}

func TestThicknessRanges(t *testing.T) {
	valid := example(t, "laser.json")
	withRanges := func(ranges []any) string {
		return edit(t, valid, func(m map[string]any) {
			for _, mat := range m["materials"].([]any) {
				if mat := mat.(map[string]any); mat["code"] == "mild_steel" {
					mat["laser_cutting"].(map[string]any)["thickness_ranges"] = ranges
				}
			}
		})
	}
	mm := func(v string) map[string]any { return map[string]any{"value": v, "unit": "mm"} }
	hole := func(v string) map[string]any {
		return map[string]any{"hole_diameter": map[string]any{"error": map[string]any{"min": mm(v)}}}
	}
	c, err := DecodeCapability([]byte(withRanges([]any{
		map[string]any{"max_thickness": mm("2"), "limits": hole("1")},
		map[string]any{"max_thickness": mm("6"), "kerf_width": mm("0.3"), "limits": hole("3")},
	})))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		thickness  measure.Length
		hole, kerf measure.Length
	}{
		{1_500_000, 1_000_000, 150_000},
		{2_000_000, 1_000_000, 150_000}, // a range's max_thickness is inclusive
		{4_000_000, 3_000_000, 300_000},
		{8_000_000, 1_500_000, 150_000}, // above every range: the material's own limits
	} {
		p, aerr := capability.Resolve(c, capability.Stock{MaterialCode: "mild_steel", Thickness: tt.thickness})
		if aerr != nil {
			t.Fatal(aerr)
		}
		if got := p.LaserCutting.HoleDiameter.Error.Min; got != tt.hole || p.LaserCutting.KerfWidth != tt.kerf {
			t.Errorf("%v thick: hole min %v kerf %v, want %v %v", tt.thickness, got, p.LaserCutting.KerfWidth, tt.hole, tt.kerf)
		}
	}

	c, err = DecodeCapability([]byte(withRanges([]any{
		map[string]any{"max_thickness": mm("6")},
		map[string]any{"max_thickness": mm("2")},
	})))
	if err == nil {
		err = capability.Validate(c)
	}
	if err == nil || err.Param != "capability.materials[0].laser_cutting.thickness_ranges[1].max_thickness" {
		t.Errorf("descending ranges: err = %v", err)
	}
}

// TestUsableAreaOverrides checks that a material and its thickness ranges override the usable area, the most specific one winning.
func TestUsableAreaOverrides(t *testing.T) {
	valid := example(t, "laser.json")
	mm := func(v string) map[string]any { return map[string]any{"value": v, "unit": "mm"} }
	area := func(w, h string) map[string]any { return map[string]any{"width": mm(w), "height": mm(h)} }
	withSteel := func(set func(lc map[string]any)) string {
		return edit(t, valid, func(m map[string]any) {
			for _, mat := range m["materials"].([]any) {
				if mat := mat.(map[string]any); mat["code"] == "mild_steel" {
					set(mat["laser_cutting"].(map[string]any))
				}
			}
		})
	}
	c, err := DecodeCapability([]byte(withSteel(func(lc map[string]any) {
		lc["usable_area"] = area("1200", "2400")
		lc["thickness_ranges"] = []any{
			map[string]any{"max_thickness": mm("6"), "usable_area": area("1000", "2000")},
		}
	})))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		material  string
		thickness measure.Length
		w, h      measure.Length
	}{
		{"mild_steel", 4_000_000, 1_000_000_000, 2_000_000_000},    // the range's
		{"mild_steel", 8_000_000, 1_200_000_000, 2_400_000_000},    // the material's
		{"stainless_304", 4_000_000, 1_500_000_000, 3_000_000_000}, // the capability's
	} {
		p, aerr := capability.Resolve(c, capability.Stock{MaterialCode: tt.material, Thickness: tt.thickness})
		if aerr != nil {
			t.Fatal(aerr)
		}
		if lc := p.LaserCutting; lc.UsableWidth != tt.w || lc.UsableHeight != tt.h {
			t.Errorf("%s %v thick: usable area %v x %v, want %v x %v", tt.material, tt.thickness, lc.UsableWidth, lc.UsableHeight, tt.w, tt.h)
		}
	}

	c, err = DecodeCapability([]byte(withSteel(func(lc map[string]any) {
		lc["thickness_ranges"] = []any{map[string]any{"max_thickness": mm("6"), "usable_area": area("0", "2000")}}
	})))
	if err == nil {
		err = capability.Validate(c)
	}
	if err == nil || err.Param != "capability.materials[0].laser_cutting.thickness_ranges[0].usable_area.width" {
		t.Errorf("zero usable width: err = %v", err)
	}
}

func TestHoleOperations(t *testing.T) {
	valid := example(t, "laser.json")
	mut := func(old, new string) string { return replace(t, valid, old, new) }
	capTests := []struct {
		name, doc, param, msg string
	}{
		{"duplicate code", mut(`"code": "tap_m3"`, `"code": "tap_m4"`), "capability.hole_operations[1].code", "listed more than once"},
		{"unknown type", mut(`"type": "tap", "name": "M3`, `"type": "drill", "name": "M3`), "capability.hole_operations[0].type", "is not one of tap, countersink, hardware"},
		{"tap without thread", mut(`"thread": { "nominal_diameter": { "value": "3", "unit": "mm" }, "pitch": { "value": "0.5", "unit": "mm" } },`, ``), "capability.hole_operations[0].thread", "is required"},
		{"unlisted material", mut(`{ "material_code": "stainless_304", "min_thickness": { "value": "1", "unit": "mm" } }`, `{ "material_code": "titanium", "min_thickness": { "value": "1", "unit": "mm" } }`), "capability.hole_operations[12].materials[0].material_code", `"titanium" is not in the capability's materials`},
		{"major below hole", mut(`"major_diameter": { "value": "6", "unit": "mm" }`, `"major_diameter": { "value": "3", "unit": "mm" }`), "capability.hole_operations[5].countersink.major_diameter", "larger than hole_diameter"},
		{"depth ratio over 1", mut(`"max_depth_ratio": "0.6"`, `"max_depth_ratio": "1.5"`), "capability.hole_operations[5].countersink.max_depth_ratio", "at most 1"},
	}
	for _, tt := range capTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeCapability([]byte(tt.doc))
			if err == nil || err.Param != tt.param || !strings.Contains(err.Message, tt.msg) {
				t.Fatalf("err = %+v, want param %s containing %q", err, tt.param, tt.msg)
			}
		})
	}
	c, err := DecodeCapability([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	asTests := []struct {
		name, doc, param, msg string
	}{
		{"valid", `[{ "operation": "tap_m4", "holes": { "diameter": { "value": "3.3", "unit": "mm" }, "part": 2 } },
			{ "operation": "csk_m4", "holes": { "entity_ids": ["2F"] }, "side": "bottom" }]`, "", ""},
		{"unknown operation", `[{ "operation": "tap_m99", "holes": { "entity_ids": ["2F"] } }]`, "hole_operations[0].operation", `"tap_m99" is not in the capability's hole_operations`},
		{"both selectors", `[{ "operation": "tap_m4", "holes": { "entity_ids": ["2F"], "diameter": { "value": "3.3", "unit": "mm" } } }]`, "hole_operations[0].holes", "exactly one of entity_ids or diameter"},
		{"no selector", `[{ "operation": "tap_m4", "holes": {} }]`, "hole_operations[0].holes", "exactly one of entity_ids or diameter"},
		{"side missing", `[{ "operation": "nut_m4_steel", "holes": { "entity_ids": ["2F"] } }]`, "hole_operations[0].side", "required for a hardware"},
		{"bad side", `[{ "operation": "csk_m4", "holes": { "entity_ids": ["2F"] }, "side": "front" }]`, "hole_operations[0].side", "is not one of top, bottom"},
		{"part zero", `[{ "operation": "tap_m4", "holes": { "entity_ids": ["2F"], "part": 0 } }]`, "hole_operations[0].holes.part", "1 or more"},
		{"part not a number", `[{ "operation": "tap_m4", "holes": { "entity_ids": ["2F"], "part": "1" } }]`, "hole_operations[0].holes.part", "whole number"},
	}
	for _, tt := range asTests {
		t.Run(tt.name, func(t *testing.T) {
			as, err := DecodeHoleAssignments([]byte(tt.doc))
			if err == nil {
				err = capability.ValidateHoleAssignments(c, as)
			}
			if tt.param == "" {
				if err != nil {
					t.Fatalf("err = %+v", err)
				}
				return
			}
			if err == nil || err.Param != tt.param || !strings.Contains(err.Message, tt.msg) {
				t.Fatalf("err = %+v, want param %s containing %q", err, tt.param, tt.msg)
			}
		})
	}
}

// TestBendAssignments checks the bends of a request on their own.
func TestBendAssignments(t *testing.T) {
	tests := []struct {
		name, doc, param, msg string
	}{
		{"valid", `[{ "entity_ids": ["30"], "angle": { "value": "135.5", "unit": "deg" }, "direction": "down", "inside_radius": { "value": "2", "unit": "mm" } }, { "entity_ids": ["31", "32"] }]`, "", ""},
		{"no entities", `[{ "entity_ids": [] }]`, "bends[0].entity_ids", ""},
		{"listed twice", `[{ "entity_ids": ["30"] }, { "entity_ids": ["31", "30"] }]`, "bends[1].entity_ids[1]", "more than once"},
		{"angle zero", `[{ "entity_ids": ["30"], "angle": { "value": "0", "unit": "deg" } }]`, "bends[0].angle.value", "greater than 0"},
		{"angle over 180", `[{ "entity_ids": ["30"], "angle": { "value": "181", "unit": "deg" } }]`, "bends[0].angle", "at most 180"},
		{"bad direction", `[{ "entity_ids": ["30"], "direction": "left" }]`, "bends[0].direction", "up, down"},
		{"radius zero", `[{ "entity_ids": ["30"], "inside_radius": { "value": "0", "unit": "mm" } }]`, "bends[0].inside_radius.value", "greater than 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeBendAssignments([]byte(tt.doc))
			if tt.param == "" {
				if err != nil {
					t.Fatalf("err = %+v", err)
				}
				return
			}
			if err == nil || err.Param != tt.param || !strings.Contains(err.Message, tt.msg) {
				t.Fatalf("err = %+v, want param %s containing %q", err, tt.param, tt.msg)
			}
		})
	}
}

// TestStockedThicknesses checks min_thickness and thicknesses: their validation, and that a thickness matches a stocked one within 0.001 mm whatever units each is written in.
func TestStockedThicknesses(t *testing.T) {
	valid := example(t, "laser.json")
	c, err := DecodeCapability([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		thickness measure.Length
		stocked   bool
	}{
		{9_525_000, true}, // 0.375 in, listed in inches
		{1_000_400, true}, // within 0.001 mm of 1 mm
		{1_002_000, false},
		{1_400_000, false},
	} {
		p, aerr := capability.Resolve(c, capability.Stock{MaterialCode: "stainless_304", Thickness: tt.thickness})
		if aerr != nil {
			t.Fatal(aerr)
		}
		if got := len(rules.ThicknessStockedRule{}.Check(nil, p)) == 0; got != tt.stocked {
			t.Errorf("%v thick: stocked = %v, want %v", tt.thickness, got, tt.stocked)
		}
	}
	steel := func(set func(lc map[string]any)) string {
		return edit(t, valid, func(m map[string]any) {
			for _, mat := range m["materials"].([]any) {
				if mat := mat.(map[string]any); mat["code"] == "stainless_304" {
					set(mat["laser_cutting"].(map[string]any))
				}
			}
		})
	}
	mm := func(v string) map[string]any { return map[string]any{"value": v, "unit": "mm"} }
	for _, tt := range []struct {
		name  string
		set   func(lc map[string]any)
		param string
	}{
		{"min above max", func(lc map[string]any) { lc["min_thickness"] = mm("20") }, ".min_thickness"},
		{"thickness below min", func(lc map[string]any) { lc["thicknesses"] = []any{mm("0.4")} }, ".thicknesses[0]"},
		{"thickness above max", func(lc map[string]any) { lc["thicknesses"] = []any{mm("1"), mm("12")} }, ".thicknesses[1]"},
		{"not increasing", func(lc map[string]any) { lc["thicknesses"] = []any{mm("2"), mm("1")} }, ".thicknesses[1]"},
	} {
		c, err := DecodeCapability([]byte(steel(tt.set)))
		if err == nil {
			err = capability.Validate(c)
		}
		if err == nil || err.Param != "capability.materials[1].laser_cutting"+tt.param {
			t.Errorf("%s: err = %v", tt.name, err)
		}
	}
}

// TestBendingCapability checks the press brake's inside radius, largest angle and K-factor: their validation, and how a material overrides them.
func TestBendingCapability(t *testing.T) {
	doc := func(bending, material string) string {
		return `{ "schema_version": "3.0", "process": "bending",
			"bending": { "max_bend_length": { "value": "1000", "unit": "mm" }` + bending + ` },
			"materials": [{ "code": "aluminum_5052", "bending": { "max_thickness": { "value": "6", "unit": "mm" }` + material + ` } }] }`
	}
	bad := []struct {
		name, bending, material, param string
	}{
		{"k_factor zero", `, "k_factor": "0"`, "", "capability.bending.k_factor"},
		{"k_factor over 0.5", `, "k_factor": "0.6"`, "", "capability.bending.k_factor"},
		{"max_angle over 180", `, "max_angle": { "value": "190", "unit": "deg" }`, "", "capability.bending.max_angle"},
		{"radius from kerf", `, "inside_radius": { "kerf_ratio": "1" }`, "", "capability.bending.inside_radius"},
		{"material k_factor", "", `, "k_factor": "-0.1"`, "capability.materials[0].bending.k_factor"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCapability([]byte(doc(tt.bending, tt.material))); err == nil || err.Param != tt.param {
				t.Fatalf("err = %+v, want param %s", err, tt.param)
			}
		})
	}
	c, err := DecodeCapability([]byte(doc(`, "k_factor": "0.4", "inside_radius": { "thickness_ratio": "1", "max": { "value": "3", "unit": "mm" } }, "max_angle": { "value": "135", "unit": "deg" }`,
		`, "inside_radius": { "thickness_ratio": "2" }, "max_angle": { "value": "120", "unit": "deg" }`)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := capability.Resolve(c, capability.Stock{MaterialCode: "aluminum_5052", Thickness: 2_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if b := p.Bending; b.InsideRadius.String() != "4" || b.MaxAngle != 120_000_000 || b.KFactor != 400_000 {
		t.Errorf("resolved radius %s, angle %d, k %d", b.InsideRadius, b.MaxAngle, b.KFactor)
	}
	p, _ = capability.Resolve(c, capability.Stock{MaterialCode: "mild_steel", Thickness: 5_000_000})
	if b := p.Bending; b.InsideRadius.String() != "3" || b.MaxAngle != 135_000_000 {
		t.Errorf("unlisted material: radius %s, angle %d", b.InsideRadius, b.MaxAngle)
	}
}

// TestDecodePlanRequest decodes a plan request, and rejects a wrong schema version inside a capability through the const tag.
func TestDecodePlanRequest(t *testing.T) {
	capJSON, err := os.ReadFile("../examples/laser.json")
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"design": {"format": "dxf"}, "stock": {"material_code": "mild_steel", "thickness": {"value": "1.5", "unit": "mm"}}, "capabilities": [` + string(capJSON) + `]}`
	r, aerr := DecodePlanRequest([]byte(doc))
	if aerr != nil {
		t.Fatal(aerr)
	}
	if len(r.Capabilities) != 1 || r.Capabilities[0].Process != constants.ProcessLaserCutting || r.Design.Format != constants.DesignFormatDXF {
		t.Errorf("decoded %+v", r)
	}
	for _, tt := range []struct {
		name, old, new, param string
	}{
		{"capability version", `"schema_version": "3.0"`, `"schema_version": "2.0"`, "capabilities[0].schema_version"},
		{"no capabilities", `[` + string(capJSON) + `]`, `[]`, "capabilities"},
	} {
		_, aerr := DecodePlanRequest([]byte(strings.Replace(doc, tt.old, tt.new, 1)))
		if aerr == nil || aerr.Param != tt.param {
			t.Errorf("%s: %v, want param %q", tt.name, aerr, tt.param)
		} else if strings.Contains(tt.name, "version") && !strings.Contains(aerr.Message, "is not supported") {
			t.Errorf("%s: message %q", tt.name, aerr.Message)
		}
	}
}

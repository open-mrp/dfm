package design

import (
	"errors"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
)

// TestStepMalformed reads broken STEP files as errors, never panics.
func TestStepMalformed(t *testing.T) {
	data := func(body string) []byte {
		return []byte("ISO-10303-21;\nHEADER;\nENDSEC;\nDATA;\n" + body + "ENDSEC;\nEND-ISO-10303-21;\n")
	}
	unit := "#90=(LENGTH_UNIT()NAMED_UNIT(*)SI_UNIT(.MILLI.,.METRE.));\n"
	for name, file := range map[string][]byte{
		"not STEP":           []byte("0\nSECTION\n"),
		"no data":            data(""),
		"no solid":           data("#1=CARTESIAN_POINT('',(0.,0.,0.));\n"),
		"point without data": data("#1=CARTESIAN_POINT('');\n#2=VERTEX_POINT('',#1);\n#9=LINE('',#1,#10);\n#10=VECTOR('',#11,1.);\n#11=DIRECTION('',(1.,0.,0.));\n#3=EDGE_CURVE('',#2,#2,#9,.T.);\n#5=ORIENTED_EDGE('',*,*,#3,.T.);\n#4=EDGE_LOOP('',(#5));\n#6=FACE_OUTER_BOUND('',#4,.T.);\n#8=PLANE('');\n#7=ADVANCED_FACE('',(#6),#8,.T.);\n#12=CLOSED_SHELL('',(#7));\n#13=MANIFOLD_SOLID_BREP('',#12);\n" + unit),
		"empty brep":         data("#13=MANIFOLD_SOLID_BREP('');\n" + unit),
		"dangling shell":     data("#13=MANIFOLD_SOLID_BREP('',#99);\n" + unit),
		"no type":            data("#1=();\n"),
		"huge multiplicity":  data("#1=CARTESIAN_POINT('',(0.,0.,0.));\n#2=B_SPLINE_CURVE_WITH_KNOTS('',2,(#1,#1,#1),.UNSPECIFIED.,.F.,.F.,(1000000000,3),(0.,1.),.UNSPECIFIED.);\n#3=VERTEX_POINT('',#1);\n#4=EDGE_CURVE('',#3,#3,#2,.T.);\n#5=ORIENTED_EDGE('',*,*,#4,.T.);\n#6=EDGE_LOOP('',(#5));\n#7=FACE_OUTER_BOUND('',#6,.T.);\n#8=ADVANCED_FACE('',(#7),#9,.T.);\n#9=B_SPLINE_SURFACE('');\n#10=CLOSED_SHELL('',(#8));\n#11=MANIFOLD_SOLID_BREP('',#10);\n" + unit),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseModel(Design{Format: constants.DesignFormatSTEP, Data: file})
			if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrUnsupportedContent) && !errors.Is(err, ErrUnitsUnknown) {
				t.Errorf("err = %v", err)
			}
		})
	}
	big := make([]byte, MaxBytes+1)
	if _, err := ParseModel(Design{Format: constants.DesignFormatSTEP, Data: big}); !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "at most 64 MB") {
		t.Errorf("oversized file: %v", err)
	}
}

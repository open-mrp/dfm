package apierror

import (
	"testing"

	"github.com/open-mrp/dfm/constants"
)

func TestEveryErrorCodeHasANextStep(t *testing.T) {
	for _, c := range constants.ErrorCode("").EnumValues() {
		if NextStep(constants.ErrorCode(c)) == "" {
			t.Errorf("%s has no next step", c)
		}
	}
}

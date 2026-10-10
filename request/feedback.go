package request

import (
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/field"
)

// ExpectedFinding describes a finding a result should (or, in a feedback fixture, should not) contain. Only the fields given are compared.
type ExpectedFinding struct {
	// Finding code.
	Code constants.FindingCode `json:"code" validate:"required"`
	// Severity of the finding.
	Severity field.Optional[constants.Severity] `json:"severity,omitzero"`
	// Entities the occurrence involves (all of these, and possibly more).
	EntityIDs field.Optional[[]string] `json:"entity_ids,omitzero"`
	// A point within 0.01 mm of the occurrence's location, in millimeters.
	Location field.Optional[PointInput] `json:"location,omitzero"`
	// The occurrence's actual value, exactly.
	Actual field.Optional[QuantityInput] `json:"actual,omitzero"`
}

// PointInput is a point in the drawing, in millimeters, as decimal strings.
type PointInput struct {
	X string `json:"x" validate:"required" format:"decimal"`
	Y string `json:"y" validate:"required" format:"decimal"`
}

// FeedbackExpectation is a feedback fixture's expect.json: what the check of its design must and must not report, so a fixed problem cannot come back. It is partial: unrelated findings are not compared. The platform's feedback report, which a user files about a stored check, is the platform's resource; a maintainer turns one into this.
type FeedbackExpectation struct {
	// The report the fixture came from (an id or link).
	Report string `json:"report" validate:"required"`
	// What was wrong, and why the expectation holds.
	Note string `json:"note" validate:"required"`
	// The verdict, when it matters.
	Verdict field.Optional[constants.Verdict] `json:"verdict,omitzero"`
	// Findings that must be present.
	Present field.Optional[[]ExpectedFinding] `json:"present,omitzero"`
	// Findings that must not be present.
	Absent field.Optional[[]ExpectedFinding] `json:"absent,omitzero"`
}

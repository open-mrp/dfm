package main

import (
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/resource"
)

// display formats a quantity for a person: "0.8 mm", "6.25 mm²", "55.25%", "90°", "36 segments". The engine's resources carry only the value and unit; formatting is the client's job, and the CLI is one.
func display(q *resource.Quantity) string {
	switch q.Unit {
	case constants.UnitSquareMillimeter:
		return q.Value + " mm²"
	case constants.UnitPercent:
		return q.Value + "%"
	case constants.UnitDegree:
		return q.Value + "°"
	case constants.UnitSegment:
		return q.Value + " segments"
	}
	return q.Value + " " + string(q.Unit)
}

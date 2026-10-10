package design

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// colorTag carries a color already resolved to "#rrggbb" or aciForeground; it is not a DXF group code.
const colorTag = -420

// aciForeground is the color of ACI 7, drawn white on a dark background and black on a light one; it matches either in a color operation.
const aciForeground = "aci7"

// aciColor returns an AutoCAD Color Index as "#rrggbb", aciForeground for 7, or "" for indexes without a fixed color here (8, 9 and the grays 250-255, whose values differ between programs).
func aciColor(i int) string {
	switch {
	case i >= 1 && i <= 6:
		return [...]string{"#ff0000", "#ffff00", "#00ff00", "#00ffff", "#0000ff", "#ff00ff"}[i-1]
	case i == 7:
		return aciForeground
	case i >= 10 && i <= 249:
		// 24 hues 15° apart, each in five shades (255, 204, 153, 127, 76), odd indexes half-way to white before shading.
		h := float64((i-10)/10) * 15
		f := math.Mod(h, 60) / 60
		var rgb [3]float64
		switch int(h / 60) {
		case 0:
			rgb = [3]float64{1, f, 0}
		case 1:
			rgb = [3]float64{1 - f, 1, 0}
		case 2:
			rgb = [3]float64{0, 1, f}
		case 3:
			rgb = [3]float64{0, 1 - f, 1}
		case 4:
			rgb = [3]float64{f, 0, 1}
		default:
			rgb = [3]float64{1, 0, 1 - f}
		}
		shade := [...]float64{255, 204, 153, 127.5, 76.5}[(i%10)/2]
		var out [3]int
		for k, v := range rgb {
			if i%2 == 1 {
				v += (1 - v) / 2
			}
			out[k] = int(math.Floor(v * shade))
		}
		return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
	}
	return ""
}

// entityColor returns the color an entity is drawn in: its true color (420), else its color index (62), else its layer's; "" when unknown.
func (f *dxfFile) entityColor(e rawEntity) string {
	if v := e.str(colorTag); v != "" {
		return v // resolved from BYBLOCK by inherited.effective
	}
	if v := e.str(420); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return fmt.Sprintf("#%06x", n&0xffffff)
		}
	}
	idx, err := e.int(62, 256)
	if err != nil {
		return ""
	}
	switch idx {
	case 256: // BYLAYER
		return f.layers[strings.ToUpper(e.layer())].color
	case 0: // BYBLOCK outside a block draws in the foreground color
		return aciForeground
	}
	return aciColor(idx)
}

// colorOperation returns the operation the capability sets for a color.
func (o Operations) colorOperation(color string) (shapeOp, bool) {
	keys := []string{color}
	if color == aciForeground {
		keys = []string{"#000000", "#ffffff"}
	}
	for _, k := range keys {
		if op, ok := o.ByColor[k]; ok {
			return operationOf(op), true
		}
	}
	return 0, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

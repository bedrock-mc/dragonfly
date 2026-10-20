package effect

import (
	"image/color"
)

// WindCharged is a lasting effect that emits a burst of wind when the entity dies. The
// effect itself does nothing yet: its trigger is not implemented.
var WindCharged windCharged

type windCharged struct {
	nopLasting
}

// RGBA ...
func (windCharged) RGBA() color.RGBA {
	return color.RGBA{R: 0xbd, G: 0xc9, B: 0xff, A: 0xff}
}

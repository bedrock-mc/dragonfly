package effect

import (
	"image/color"
)

// Weaving is a lasting effect that spreads cobwebs around the entity when it dies. The
// effect itself does nothing yet: its trigger is not implemented.
var Weaving weaving

type weaving struct {
	nopLasting
}

// RGBA ...
func (weaving) RGBA() color.RGBA {
	return color.RGBA{R: 0x78, G: 0x69, B: 0x5a, A: 0xff}
}

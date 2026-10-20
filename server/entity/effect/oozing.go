package effect

import (
	"image/color"
)

// Oozing is a lasting effect that spawns slimes around the entity when it dies. The
// effect itself does nothing yet: its trigger is not implemented.
var Oozing oozing

type oozing struct {
	nopLasting
}

// RGBA ...
func (oozing) RGBA() color.RGBA {
	return color.RGBA{R: 0x99, G: 0xff, B: 0xa3, A: 0xff}
}

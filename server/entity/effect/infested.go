package effect

import (
	"image/color"
)

// Infested is a lasting effect that sometimes releases silverfish when the entity is hurt. The
// effect itself does nothing yet: its trigger is not implemented.
var Infested infested

type infested struct {
	nopLasting
}

// RGBA ...
func (infested) RGBA() color.RGBA {
	return color.RGBA{R: 0x8c, G: 0x9b, B: 0x8c, A: 0xff}
}

package effect

import (
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/world"
)

// pulseTarget records health changes without simulating movement or combat immunity.
type pulseTarget struct {
	living
	health float64
}

// Health exposes the target's current health to poison admission.
func (p *pulseTarget) Health() float64 { return p.health }

// Heal records the periodic healing amount.
func (p *pulseTarget) Heal(n float64, _ world.HealingSource) float64 { p.health += n; return n }

// Hurt records the periodic damage amount.
func (p *pulseTarget) Hurt(n float64, _ world.DamageSource) (float64, bool) {
	p.health -= n
	return n, true
}

func TestFiniteEffectPulsesRetainRemainingDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typ      LastingType
		interval int
		delta    float64
	}{
		{"regeneration", Regeneration, 50, 1},
		{"poison", Poison, 25, -1},
		{"wither", Wither, 40, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pulseTarget{health: 10}
			e := New(tc.typ, 1, time.Duration(tc.interval+15)*time.Second/20)
			for range 15 {
				tc.typ.Apply(p, e)
				e = e.TickDuration()
				if p.health != 10 {
					t.Fatal("periodic effect applied before its remaining interval")
				}
			}
			tc.typ.Apply(p, e)
			if p.health != 10+tc.delta {
				t.Fatalf("health = %v, want %v", p.health, 10+tc.delta)
			}
		})
	}
}

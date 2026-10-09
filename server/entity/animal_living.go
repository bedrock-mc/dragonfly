package entity

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

// Animal is a transaction-scoped passive living actor. Its state belongs to its
// EntityHandle and survives transaction wrappers, chunk unloading and reopening.
type Animal struct{ *Ent }

var _ Living = (*Animal)(nil)

type animalState struct {
	BaseBehaviour
	health                *HealthManager
	effects               *EffectManager
	movement              MovementComputer
	surface, natural      bool
	babyUntil             time.Duration
	speed                 float64
	immuneUntil, deathAge time.Duration
	lastDamage            float64
}

// state returns the living state attached to the persistent actor handle.
func (a *Animal) state() *animalState { return a.data.Data.(*animalState) }

// Health returns current health.
func (a *Animal) Health() float64 { return a.state().health.Health() }

// MaxHealth returns the health limit.
func (a *Animal) MaxHealth() float64 { return a.state().health.MaxHealth() }

// SetMaxHealth changes the health limit, clamping current health to it.
func (a *Animal) SetMaxHealth(v float64) {
	if !math.IsNaN(v) && !math.IsInf(v, 0) {
		a.state().health.SetMaxHealth(v)
	}
}

// Dead reports whether this actor has lost all health.
func (a *Animal) Dead() bool { return a.Health() <= 0 }

// Speed returns the movement attribute.
func (a *Animal) Speed() float64 { return a.state().speed }

// SetSpeed changes the movement attribute to a finite nonnegative value.
func (a *Animal) SetSpeed(v float64) {
	if v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
		a.state().speed = v
	}
}

// Heal restores health without resurrecting dead actors.
func (a *Animal) Heal(v float64, _ world.HealingSource) float64 {
	if a.Dead() || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	before := a.Health()
	a.state().health.AddHealth(v)
	return a.Health() - before
}

// Hurt applies damage and simulation-time attack immunity, then starts death.
func (a *Animal) Hurt(v float64, src world.DamageSource) (float64, bool) {
	if a.Dead() || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	b := a.state()
	if _, ok := b.effects.Effect(effect.FireResistance); ok && src.Fire() {
		return 0, false
	}
	damage := v
	if a.Age() < b.immuneUntil {
		damage -= b.lastDamage
		if damage <= 0 {
			return 0, false
		}
	}
	b.immuneUntil, b.lastDamage = a.Age()+time.Second/2, v
	damage = min(damage, a.Health())
	b.health.AddHealth(-damage)
	action := world.EntityAction(HurtAction{})
	if a.Dead() {
		b.deathAge = a.Age()
		action = DeathAction{}
	}
	for _, viewer := range a.tx.Viewers(a.Position()) {
		viewer.ViewEntityAction(a, action)
	}
	return damage, true
}

// KnockBack applies horizontal impulse and an upward impulse safely at zero distance.
func (a *Animal) KnockBack(src mgl64.Vec3, force, height float64) {
	delta := a.Position().Sub(src)
	delta[1] = 0
	if delta.LenSqr() > 0 {
		delta = delta.Normalize().Mul(force)
	}
	v := a.Velocity().Mul(.5).Add(delta)
	v[1] = height
	a.SetVelocity(v)
}

// AddEffect applies an effect to this living actor.
func (a *Animal) AddEffect(e effect.Effect) { a.state().effects.Add(e, a); a.updateState() }

// RemoveEffect ends an active effect.
func (a *Animal) RemoveEffect(e effect.Type) { a.state().effects.Remove(e, a); a.updateState() }

// Effects returns the active effect snapshot.
func (a *Animal) Effects() []effect.Effect { return a.state().effects.Effects() }

// Baby reports whether the animal is still growing.
func (a *Animal) Baby() bool { return a.Age() < a.state().babyUntil }

// Scale returns the adult or baby geometry scale.
func (a *Animal) Scale() float64 {
	if a.Baby() {
		return .5
	}
	return 1
}

// Tick advances living effects, movement and the terminal death lifecycle.
func (a *Animal) Tick(tx *world.Tx, tick int64) {
	b := a.state()
	if a.Dead() {
		if a.Age()-b.deathAge >= time.Second {
			_ = a.Close()
			return
		}
	} else {
		b.effects.Tick(a, tx)
	}
	baby := a.Baby()
	a.Ent.Tick(tx, tick)
	if baby && !a.Baby() {
		a.updateState()
	}
}

// Explode routes explosion damage through health before applying impulse.
func (a *Animal) Explode(src world.ExplosionSource, impact float64) {
	if impact <= 0 {
		return
	}
	a.Hurt((impact*impact+impact)*7, ExplosionDamageSource{Source: src})
	a.KnockBack(src.Position(), impact, impact)
}

// Tick moves the persistent actor using the world's block collision system.
func (b *animalState) Tick(e *Ent, tx *world.Tx) *Movement {
	m := b.movement.TickMovement(e, e.data.Pos, e.data.Vel, e.data.Rot, tx)
	e.data.Pos, e.data.Vel = m.Position(), m.Velocity()
	return m
}

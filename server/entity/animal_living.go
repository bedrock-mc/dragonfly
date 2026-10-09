package entity

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
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
	fireElapsed           time.Duration
	extinguished          bool
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
		a.tx.MarkEntityModified(a)
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
		a.tx.MarkEntityModified(a)
		a.state().speed = v
	}
}

// Heal restores health without resurrecting dead actors.
func (a *Animal) Heal(v float64, _ world.HealingSource) float64 {
	if a.Dead() || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	before := a.Health()
	a.tx.MarkEntityModified(a)
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
	if res, ok := b.effects.Effect(effect.Resistance); ok {
		v *= effect.Resistance.Multiplier(src, res.Level())
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
	a.tx.MarkEntityModified(a)
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

// Invisible reports the visibility supplied by active potion effects.
func (a *Animal) Invisible() bool { _, ok := a.state().effects.Effect(effect.Invisibility); return ok }

// SetOnFire starts or extends burning without resetting an active damage interval.
func (a *Animal) SetOnFire(d time.Duration) {
	b := a.state()
	if d <= 0 {
		b.fireElapsed = 0
		b.extinguished = true
	} else if a.OnFireDuration() <= 0 {
		b.fireElapsed = 0
	}
	a.Ent.SetOnFire(d)
}

// Extinguish ends burning and discards its pending damage interval.
func (a *Animal) Extinguish() { a.SetOnFire(0) }

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
	burning := a.OnFireDuration() > 0
	b.extinguished = false
	if a.Ent.tick(tx, tick) {
		return
	}
	if !a.Dead() && burning && !b.extinguished {
		if tx.RainingAt(cube.PosFromVec3(a.Position())) {
			a.Extinguish()
		} else {
			b.fireElapsed += time.Second / 20
			if b.fireElapsed >= time.Second {
				b.fireElapsed -= time.Second
				a.Hurt(1, block.FireDamageSource{})
			}
		}
	}
	if baby && !a.Baby() {
		a.updateState()
	}
}

// Explode routes explosion damage through health before applying impulse.
func (a *Animal) Explode(src world.ExplosionSource, impact float64) {
	if impact <= 0 {
		return
	}
	a.Hurt(block.ExplosionDamage(src.Size(), impact), ExplosionDamageSource{Source: src})
	a.KnockBack(src.Position(), impact, impact)
}

// Tick moves the persistent actor using the world's block collision system.
func (b *animalState) Tick(e *Ent, tx *world.Tx) *Movement {
	m := b.movement.TickMovement(&Animal{Ent: e}, e.data.Pos, e.data.Vel, e.data.Rot, tx)
	e.data.Pos, e.data.Vel = m.Position(), m.Velocity()
	a := &Animal{Ent: e}
	if !a.Dead() {
		a.checkInsiders(tx)
		if b.movement.OnGround() {
			a.checkSteppers(tx)
		}
	}
	return m
}

// checkInsiders dispatches environmental contacts through the living wrapper.
// Portal contacts are dispatched once by Ent's terminal travel handling.
func (a *Animal) checkInsiders(tx *world.Tx) {
	box := a.H().Type().BBox(a).Translate(a.Position()).Grow(-.0001)
	for pos := range cube.Range3D(cube.PosFromVec3(box.Min()), cube.PosFromVec3(box.Max())) {
		b := tx.Block(pos)
		if _, portal := b.(portalBlock); portal {
			continue
		}
		if inside, ok := b.(block.EntityInsider); ok {
			inside.EntityInside(pos, tx, a)
			if _, liquid := b.(world.Liquid); liquid {
				continue
			}
		}
		if liquid, ok := tx.Liquid(pos); ok {
			if inside, ok := liquid.(block.EntityInsider); ok {
				inside.EntityInside(pos, tx, a)
			}
		}
	}
}

// checkSteppers dispatches the supporting block's standing contact when grounded.
func (a *Animal) checkSteppers(tx *world.Tx) {
	box := a.H().Type().BBox(a).Translate(a.Position()).Grow(-.0001)
	low, high := cube.PosFromVec3(box.Min().Sub(mgl64.Vec3{0, .05, 0})), cube.PosFromVec3(box.Max())
	for x := low[0]; x <= high[0]; x++ {
		for z := low[2]; z <= high[2]; z++ {
			pos := cube.Pos{x, low[1], z}
			if step, ok := tx.Block(pos).(block.EntityStepper); ok {
				step.EntityStepOn(pos, tx, a)
				return
			}
		}
	}
}

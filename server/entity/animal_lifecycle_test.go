package entity

import (
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"math"
	"testing"
	"time"
)

// animalMetadataViewer observes state updates through the public entity contract.
type animalMetadataViewer struct {
	world.NopViewer
	baby    bool
	effects int
	name    string
	fire    time.Duration
}

// ViewEntityState records observable living metadata delivered to a viewer.
func (v *animalMetadataViewer) ViewEntityState(e world.Entity) {
	a, ok := e.(*Animal)
	if !ok {
		v.baby = false
		return
	}
	v.baby = a.Baby()
	v.effects = len(a.Effects())
	v.name = a.NameTag()
	v.fire = a.OnFireDuration()
}

func TestAnimalBabyMovementAndMetadata(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	v := &animalMetadataViewer{}
	loader := world.NewLoader(1, w, v)
	w.Do(func(tx *world.Tx) {
		tx.SetBlock(cube.Pos{0, 9, 0}, block.Stone{}, nil)
		tx.SetBlock(cube.Pos{1, 9, 0}, block.Stone{}, nil)
		tx.SetBlock(cube.Pos{1, 11, 0}, block.Stone{}, nil)
		loader.Move(tx, mgl64.Vec3{.5, 10, .5})
		loader.Load(tx, 10)
		a := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{.5, 10, .5}}).New(CowType, naturalAnimalConfig{species: CowType, baby: true})).(*Animal)
		a.SetNameTag("Baby")
		a.SetOnFire(time.Second)
		a.AddEffect(effect.New(effect.Speed, 1, time.Second))
		if !v.baby || v.effects != 1 || v.name != "Baby" || v.fire != time.Second {
			t.Fatalf("metadata lost outer living state: %+v", v)
		}
		a.SetVelocity(mgl64.Vec3{.8, 0, 0})
		a.Tick(tx, 1)
		if a.Position()[0] <= 1 {
			t.Fatalf("baby hit an adult-only overhead obstruction: %v", a.Position())
		}
		loader.Close(tx)
	})
}

func TestAnimalEffectsAndFullAgeRoundTrip(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.data.Age = 2 * time.Hour
		a.AddEffect(effect.New(effect.Speed, 1, time.Second))
		a.AddEffect(effect.New(effect.HealthBoost, 1, time.Second))
		encoded, err := nbt.Marshal(CowType.EncodeNBT(a.data))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err = nbt.Unmarshal(encoded, &m); err != nil {
			t.Fatal(err)
		}
		data := world.EntityData{Age: 32767 * time.Second / 20}
		CowType.DecodeNBT(m, &data)
		restored := &Animal{Ent: Open(tx, a.H(), &data)}
		if restored.Age() != 2*time.Hour || len(restored.Effects()) != 2 || math.Abs(restored.Speed()-.3) > 1e-9 || restored.MaxHealth() != 14 {
			t.Fatalf("lost persisted age/effects: age=%v effects=%v speed=%v max=%v", restored.Age(), restored.Effects(), restored.Speed(), restored.MaxHealth())
		}
		for range 21 {
			restored.state().effects.Tick(restored, tx)
		}
		if math.Abs(restored.Speed()-.25) > 1e-9 || restored.MaxHealth() != 10 {
			t.Fatalf("temporary modifiers survived expiration: speed=%v max=%v", restored.Speed(), restored.MaxHealth())
		}
		restored.Hurt(100, VoidDamageSource{})
		encoded, _ = nbt.Marshal(CowType.EncodeNBT(restored.data))
		if err = nbt.Unmarshal(encoded, &m); err != nil {
			t.Fatal(err)
		}
		data = world.EntityData{Age: 32767 * time.Second / 20}
		CowType.DecodeNBT(m, &data)
		restored = &Animal{Ent: Open(tx, a.H(), &data)}
		for range 21 {
			restored.Tick(tx, 1)
		}
		if !a.H().Closed() {
			t.Fatal("old saved corpse did not complete its death timer")
		}
	})
}

func TestAnimalResistanceAndExplosionDamage(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.AddEffect(effect.New(effect.Resistance, 4, time.Minute))
		if n, _ := a.Hurt(5, AttackDamageSource{}); math.Abs(n-1) > 1e-9 {
			t.Fatalf("resistance damage=%v", n)
		}
		b := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		b.Explode(world.EntityExplosionSource{Entity: a, ExplosionSize: 4}, .5)
		if !b.Dead() {
			t.Fatalf("cow survived lethal strength-four explosion with health %v", b.Health())
		}
	})
}

func TestAnimalPendingEffectExpirationAndZeroSpeed(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.AddEffect(effect.New(effect.Speed, 1, 25*time.Millisecond))
		a.state().effects.Tick(a, tx)
		encoded, _ := nbt.Marshal(CowType.EncodeNBT(a.data))
		var m map[string]any
		if err := nbt.Unmarshal(encoded, &m); err != nil {
			t.Fatal(err)
		}
		data := world.EntityData{}
		CowType.DecodeNBT(m, &data)
		restored := &Animal{Ent: Open(tx, a.H(), &data)}
		restored.state().effects.Tick(restored, tx)
		if math.Abs(restored.Speed()-.25) > 1e-9 {
			t.Errorf("expired saved speed modifier remained: %v", restored.Speed())
		}
		a.SetSpeed(0)
		m = CowType.EncodeNBT(a.data)
		CowType.DecodeNBT(m, &data)
		restored = &Animal{Ent: Open(tx, a.H(), &data)}
		if restored.Speed() != 0 {
			t.Errorf("saved zero speed became %v", restored.Speed())
		}
	})
}

func TestAnimalEnvironmentalDamageAndBabyEmbeddedBounds(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{.5, 10, .5}}).New(CowType, naturalAnimalConfig{species: CowType, baby: true})).(*Animal)
		if CowType.BBox(a.Ent).Height() != CowType.BBox(a).Height() {
			t.Error("embedded portal bounds use adult dimensions")
		}
		tx.SetLiquid(cube.Pos{0, 10, 0}, block.Lava{Depth: 8})
		a.Tick(tx, 1)
		if a.Health() >= a.MaxHealth() || a.OnFireDuration() <= 0 {
			t.Error("lava did not hurt or ignite the living actor")
		}
		b := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{10, 10, 0}})).(*Animal)
		b.SetOnFire(2 * time.Second)
		for range 20 {
			b.Tick(tx, 1)
		}
		if b.Health() >= b.MaxHealth() {
			t.Error("burning did no periodic damage")
		}
		tx.SetLiquid(cube.Pos{10, 8, 0}, block.Water{Depth: 8})
		b.Teleport(mgl64.Vec3{10, 8, 0})
		b.SetVelocity(mgl64.Vec3{})
		b.Tick(tx, 1)
		if b.OnFireDuration() != 0 {
			t.Error("water did not extinguish actor")
		}
	})
}

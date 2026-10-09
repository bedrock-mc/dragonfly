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

func TestAnimalStandingOnMagma(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		tx.SetBlock(cube.Pos{0, 9, 0}, block.Magma{}, nil)
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{.5, 10, .5}})).(*Animal)
		a.Tick(tx, 1)
		if a.Health() >= a.MaxHealth() {
			t.Fatal("standing on magma caused no damage")
		}
	})
}

func TestAnimalPortalTickEndsBeforeFireDamage(t *testing.T) {
	var source, destination *world.World
	source = world.Config{Synchronous: true, Entities: DefaultRegistry, PortalDestination: func(dim world.Dimension) *world.World { return destination }}.New()
	defer source.Close()
	destination = world.Config{Synchronous: true, Dim: world.Nether, Entities: DefaultRegistry}.New()
	defer destination.Close()
	sourcePos, targetPos := cube.Pos{80, 64, 80}, cube.Pos{10, 64, 10}
	destination.Do(func(tx *world.Tx) { buildActivePortal(tx, targetPos) })
	var handle *world.EntityHandle
	source.Do(func(tx *world.Tx) {
		buildActivePortal(tx, sourcePos)
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: sourcePos.Vec3Middle()})).(*Animal)
		handle = a.H()
		a.state().PortalTravelComputer().Instantaneous = func(world.Dimension, world.Dimension) bool { return true }
		a.SetOnFire(time.Second + time.Second/20)
		a.Tick(tx, 1)
		// The destination owner may run after this transaction; capture no shared state here.
	})
	// Wait for destination ownership before observing persistent state.
	waitForEntityWorld(t, handle, destination)
	var health float64
	destination.Do(func(tx *world.Tx) {
		for e := range tx.Entities() {
			health = e.(*Animal).Health()
		}
	})
	if health != 10 {
		t.Fatalf("terminal portal tick applied source-world fire damage: health=%v", health)
	}
}

// animalDespawnObserver verifies the living contract at terminal removal.
type animalDespawnObserver struct {
	world.NopHandler
	living bool
}

// HandleEntityDespawn records whether the removed actor retains living state.
func (h *animalDespawnObserver) HandleEntityDespawn(_ *world.Tx, e world.Entity) {
	_, h.living = e.(Living)
}

func TestAnimalInvisibilityDespawnAndFractionalFire(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	h := &animalDespawnObserver{}
	w.Handle(h)
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.AddEffect(effect.New(effect.Invisibility, 1, time.Minute))
		visibility, ok := any(a).(interface{ Invisible() bool })
		if !ok || !visibility.Invisible() {
			t.Error("invisibility effect has no metadata visibility")
		}
		a.SetOnFire(2025 * time.Millisecond)
		for range 41 {
			a.Tick(tx, 1)
		}
		if a.Health() >= 10 {
			t.Error("fractional fire duration caused no burning damage")
		}
		a.Close()
		if !h.living {
			t.Error("despawn callback lost living wrapper")
		}
	})
}

func TestAnimalPeriodicEffectsRetainPulseProgress(t *testing.T) {
	for name, typ := range map[string]effect.LastingType{"regeneration": effect.Regeneration, "poison": effect.Poison, "wither": effect.Wither} {
		t.Run(name, func(t *testing.T) {
			w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
			defer w.Close()
			w.Do(func(tx *world.Tx) {
				a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
				a.Hurt(3, VoidDamageSource{})
				a.AddEffect(effect.New(typ, 1, time.Minute))
				a.state().effects.Tick(a, tx)
				encoded, err := nbt.Marshal(CowType.EncodeNBT(a.data))
				if err != nil {
					t.Fatal(err)
				}
				var m map[string]any
				if err = nbt.Unmarshal(encoded, &m); err != nil {
					t.Fatal(err)
				}
				data := world.EntityData{}
				CowType.DecodeNBT(m, &data)
				restored := &Animal{Ent: Open(tx, a.H(), &data)}
				// Let damage immunity expire in both copies without advancing the effect.
				a.data.Age += time.Second
				restored.data.Age += time.Second
				a.state().effects.Tick(a, tx)
				restored.state().effects.Tick(restored, tx)
				if a.Health() != restored.Health() {
					t.Fatalf("reopening changed periodic pulse: uninterrupted=%v reopened=%v", a.Health(), restored.Health())
				}
			})
		})
	}
}

func TestAnimalDamageImmunitySurvivesReopening(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.data.Age = time.Hour
		a.Hurt(3, AttackDamageSource{})
		encoded, err := nbt.Marshal(CowType.EncodeNBT(a.data))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err = nbt.Unmarshal(encoded, &m); err != nil {
			t.Fatal(err)
		}
		data := world.EntityData{}
		CowType.DecodeNBT(m, &data)
		restored := &Animal{Ent: Open(tx, a.H(), &data)}
		if n, _ := restored.Hurt(3, AttackDamageSource{}); n != 0 {
			t.Fatalf("equal attack bypassed saved immunity: %v", n)
		}
		if n, _ := restored.Hurt(4, AttackDamageSource{}); n != 1 {
			t.Fatalf("stronger attack lost prior damage: %v", n)
		}
		restored.data.Age += time.Second / 2
		if n, _ := restored.Hurt(3, AttackDamageSource{}); n != 3 {
			t.Fatalf("saved immunity did not expire in simulation time: %v", n)
		}
	})
}

func TestAnimalEffectsDirectMapRoundTrip(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})).(*Animal)
		a.AddEffect(effect.New(effect.Speed, 1, time.Second))
		a.AddEffect(effect.New(effect.HealthBoost, 1, time.Second))
		data := world.EntityData{}
		CowType.DecodeNBT(CowType.EncodeNBT(a.data), &data)
		restored := &Animal{Ent: Open(tx, a.H(), &data)}
		if len(restored.Effects()) != 2 {
			t.Fatal("direct-map reopening dropped lasting effects")
		}
		for range 21 {
			restored.state().effects.Tick(restored, tx)
		}
		if math.Abs(restored.Speed()-.25) > 1e-9 || restored.MaxHealth() != 10 {
			t.Fatalf("direct-map reopening retained expired modifiers: speed=%v max=%v", restored.Speed(), restored.MaxHealth())
		}
	})
}

// physicsGenerationWall installs collision terrain when its column is initialized.
type physicsGenerationWall struct{ world.NopHandler }

// HandleChunkGenerate places a wall in the previously unloaded neighbor.
func (physicsGenerationWall) HandleChunkGenerate(tx *world.Tx, pos world.ChunkPos) {
	if pos == (world.ChunkPos{1, 0}) {
		for y := 10; y <= 11; y++ {
			tx.SetBlock(cube.Pos{16, y, 0}, block.Stone{}, &world.SetOpts{DisableBlockUpdates: true, DisableRedstoneUpdates: true})
		}
	}
}

func TestAnimalPhysicsReadsInitializedNeighbor(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: DefaultRegistry}.New()
	defer w.Close()
	w.Handle(physicsGenerationWall{})
	var cow *world.EntityHandle
	w.Do(func(tx *world.Tx) {
		tx.SetBlock(cube.Pos{15, 9, 0}, block.Stone{}, &world.SetOpts{DisableBlockUpdates: true, DisableRedstoneUpdates: true})
		a := tx.AddEntity(NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{15.3, 10, .5}})).(*Animal)
		cow = a.H()
		a.SetVelocity(mgl64.Vec3{.8, 0, 0})
		if tx.ChunkLoaded(world.ChunkPos{1, 0}) {
			t.Fatal("collision neighbor was already loaded")
		}
	})
	w.AdvanceTick()
	w.Do(func(tx *world.Tx) {
		a, ok := cow.Entity(tx)
		if !ok {
			t.Fatal("cow disappeared")
		}
		if x := a.Position()[0]; x > 15.551 {
			t.Fatalf("cow crossed wall before neighbor initialization: x=%v", x)
		}
	})
}

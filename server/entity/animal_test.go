package entity_test

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/mcdb"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
	"slices"
	"testing"
)

func TestDefaultRegistryIncludesPassiveSpecies(t *testing.T) {
	for _, name := range []string{"minecraft:cow", "minecraft:pig", "minecraft:sheep", "minecraft:chicken"} {
		if _, ok := entity.DefaultRegistry.Lookup(name); !ok {
			t.Errorf("passive species %s cannot be instantiated or reopened", name)
		}
	}
}

func TestAnimalPersistsAcrossMovementAndReopening(t *testing.T) {
	dir := t.TempDir()
	open := func() *world.World {
		db, err := mcdb.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return world.Config{Synchronous: true, Provider: db, Entities: entity.DefaultRegistry}.New()
	}
	w := open()
	var id uuid.UUID
	w.Do(func(tx *world.Tx) {
		a := tx.AddEntity(entity.NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{.49, 10, .49}, NameTag: "Saved cow"})).(*entity.Animal)
		id = a.H().UUID()
		a.Hurt(3, entity.AttackDamageSource{})
		// Save an initial source reference before crossing a column boundary.
	})
	w.Save()
	w.Do(func(tx *world.Tx) {
		tx.Block(cube.Pos{16, 10, 0})
		for e := range tx.Entities() {
			e.(*entity.Animal).Teleport(mgl64.Vec3{16.49, 10, .49})
		}
	})
	w.AdvanceTick()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w = open()
	w.Do(func(tx *world.Tx) {
		tx.Block(cube.Pos{0, 10, 0})
		tx.Block(cube.Pos{16, 10, 0})
		actors := slices.Collect(tx.Entities())
		if len(actors) != 1 {
			t.Fatalf("reopened actors = %d, want one moved cow", len(actors))
		}
		a := actors[0].(*entity.Animal)
		if a.H().UUID() != id || a.Health() != 7 || a.NameTag() != "Saved cow" || a.Position()[0] < 16 {
			t.Fatalf("reopened cow lost its identity/state: id=%v health=%v name=%q pos=%v", a.H().UUID(), a.Health(), a.NameTag(), a.Position())
		}
		if _, ok := a.Hurt(100, entity.VoidDamageSource{}); !ok {
			t.Fatal("cow cannot take terminal damage")
		}
	})
	for range 21 {
		w.AdvanceTick()
	}
	w.Do(func(tx *world.Tx) {
		for range tx.Entities() {
			t.Fatal("dead cow still owns a world actor")
		}
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w = open()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		tx.Block(cube.Pos{16, 10, 0})
		for range tx.Entities() {
			t.Fatal("removed cow was resurrected by reopening")
		}
	})
}

func TestAnimalDamageImmunityUsesSimulationTime(t *testing.T) {
	w := world.Config{Synchronous: true, Entities: entity.DefaultRegistry}.New()
	defer w.Close()
	var h *world.EntityHandle
	w.Do(func(tx *world.Tx) {
		h = entity.NewSheep(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})
		a := tx.AddEntity(h).(*entity.Animal)
		a.Hurt(2, entity.AttackDamageSource{})
		if n, ok := a.Hurt(2, entity.AttackDamageSource{}); ok || n != 0 {
			t.Fatal("same-tick equal attack bypassed immunity")
		}
		if n, ok := a.Hurt(3, entity.AttackDamageSource{}); !ok || n != 1 {
			t.Fatal("stronger immune-period attack did not apply only the excess")
		}
	})
	for range 10 {
		w.AdvanceTick()
	}
	h.Do(func(tx *world.Tx, e world.Entity) {
		a := e.(*entity.Animal)
		if n, ok := a.Hurt(1, entity.AttackDamageSource{}); !ok || n != 1 {
			t.Fatal("simulation ticks did not expire immunity")
		}
	})
}

func TestAnimalMutationPersistsWithoutTick(t *testing.T) {
	dir := t.TempDir()
	open := func() *world.World {
		db, err := mcdb.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return world.Config{Synchronous: true, Provider: db, Entities: entity.DefaultRegistry}.New()
	}
	w := open()
	w.Do(func(tx *world.Tx) { tx.AddEntity(entity.NewCow(world.EntitySpawnOpts{Position: mgl64.Vec3{0, 10, 0}})) })
	w.Close()
	w = open()
	w.Do(func(tx *world.Tx) {
		tx.Block(cube.Pos{0, 10, 0})
		for e := range tx.Entities() {
			a := e.(*entity.Animal)
			a.Hurt(3, entity.AttackDamageSource{})
			a.SetSpeed(0)
		}
	})
	w.Close()
	w = open()
	defer w.Close()
	w.Do(func(tx *world.Tx) {
		tx.Block(cube.Pos{0, 10, 0})
		for e := range tx.Entities() {
			a := e.(*entity.Animal)
			if a.Health() != 7 || a.Speed() != 0 {
				t.Fatalf("unticked saved mutation lost: health=%v speed=%v", a.Health(), a.Speed())
			}
		}
	})
}

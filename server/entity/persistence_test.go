package entity

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/mcdb"
	"github.com/go-gl/mathgl/mgl64"
)

func TestSupportedItemChangesSurviveSave(t *testing.T) {
	for _, mode := range []string{"name", "tick", "move"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			open := func() *world.World {
				db, err := mcdb.Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				return world.Config{Provider: db, Entities: DefaultRegistry, Synchronous: true}.New()
			}
			w := open()
			w.Do(func(tx *world.Tx) {
				tx.AddEntity(NewItem(world.EntitySpawnOpts{Position: mgl64.Vec3{1, 10, 1}}, item.NewStack(item.Diamond{}, 1)))
			})
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			w = open()
			w.Do(func(tx *world.Tx) {
				tx.Block(cube.Pos{1, 10, 1})
				if mode == "move" {
					tx.Block(cube.Pos{20, 10, 1})
				}
				for e := range tx.Entities() {
					if mode == "name" {
						e.(*Ent).SetNameTag("Saved item")
					}
					if mode == "move" {
						e.(*Ent).Teleport(mgl64.Vec3{20, 10, 1})
					}
				}
			})
			if mode != "name" {
				w.AdvanceTick()
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			w = open()
			defer w.Close()
			w.Do(func(tx *world.Tx) {
				tx.Block(cube.Pos{1, 10, 1})
				tx.Block(cube.Pos{20, 10, 1})
				count := 0
				for e := range tx.Entities() {
					count++
					it := e.(*Ent)
					if mode == "name" && it.NameTag() != "Saved item" {
						t.Fatal("name change was not saved")
					}
					if mode == "tick" && it.Age() == 0 {
						t.Fatal("ticked item reverted to its saved age")
					}
					if mode == "move" && it.Position()[0] < 16 {
						t.Fatal("cross-chunk teleport was not saved")
					}
				}
				if count != 1 {
					t.Fatalf("reopened item count = %d, want 1", count)
				}
			})
		})
	}
}

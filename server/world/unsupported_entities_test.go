package world_test

import (
	"reflect"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/df-mc/dragonfly/server/world/mcdb"
)

func TestUnsupportedEntitiesSurviveWorldSave(t *testing.T) {
	dir := t.TempDir()
	db, err := mcdb.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	actors := make([]chunk.Entity, 0, 4)
	for i, name := range []string{"minecraft:cow", "minecraft:pig", "minecraft:sheep", "minecraft:chicken"} {
		actors = append(actors, chunk.Entity{ID: int64(i + 42), Data: map[string]any{
			"identifier": name, "Health": float32(7), "Age": int32(-120),
		}})
	}
	col := &chunk.Column{Chunk: chunk.New(world.DefaultBlockRegistry, world.Overworld.Range()), Entities: actors}
	if err := db.StoreColumn(world.ChunkPos{}, world.Overworld, col); err != nil {
		t.Fatal(err)
	}
	w := world.Config{Provider: db, Entities: world.EntityRegistryConfig{}.New(nil), Synchronous: true}.New()
	w.Do(func(tx *world.Tx) {
		tx.SetBlock(cube.Pos{0, 4, 0}, block.Stone{}, nil)
		for e := range tx.Entities() {
			t.Errorf("unsupported actor became live: %s", e.H().Type().EncodeEntity())
		}
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = mcdb.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.LoadColumn(world.ChunkPos{}, world.Overworld)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Entities, actors) {
		t.Fatalf("saved unsupported actors = %#v, want %#v", got.Entities, actors)
	}
	if got.Chunk.Block(0, 4, 0, 0) != world.DefaultBlockRegistry.BlockRuntimeID(block.Stone{}) {
		t.Fatal("ordinary terrain edit was not saved")
	}
}

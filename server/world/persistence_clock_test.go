package world

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// columnSaveObserver records the serialized columns delivered to a provider.
type columnSaveObserver struct {
	NopProvider
	stored map[ChunkPos]*chunk.Column
}

// StoreColumn records the terrain and entity snapshot requested by the world.
func (p *columnSaveObserver) StoreColumn(pos ChunkPos, _ Dimension, c *chunk.Column) error {
	p.stored[pos] = c
	return nil
}

func TestViewedGeneratedTerrainIsSaved(t *testing.T) {
	for _, mode := range []string{"save", "unload", "close"} {
		t.Run(mode, func(t *testing.T) {
			p := &columnSaveObserver{stored: make(map[ChunkPos]*chunk.Column)}
			w := Config{Synchronous: true, Provider: p}.New()
			defer w.Close()
			w.Do(func(tx *Tx) { tx.Block(cube.Pos{}) })
			switch mode {
			case "save":
				w.Save()
			case "unload":
				w.Do(func(tx *Tx) { w.closeChunk(tx, ChunkPos{}, w.chunks[ChunkPos{}]) })
			case "close":
				w.Close()
			}
			if p.stored[ChunkPos{}] == nil {
				t.Fatal("viewed generated terrain was not persisted")
			}
		})
	}
}

// delayedUpdateBlock records when an entity's delayed update becomes observable.
type delayedUpdateBlock struct{ fired *bool }

// EncodeBlock identifies the isolated scheduled-update fixture.
func (delayedUpdateBlock) EncodeBlock() (string, map[string]any) { return "test:delayed_update", nil }

// Hash identifies the fixture through its registered block state.
func (delayedUpdateBlock) Hash() (uint64, uint64) { return 1<<32 - 1, 0 }

// Model returns the fixture's collision model.
func (delayedUpdateBlock) Model() BlockModel { return unknownModel{} }

// ScheduledTick records the delivered update.
func (b delayedUpdateBlock) ScheduledTick(cube.Pos, *Tx, *rand.Rand) { *b.fired = true }

// delayedUpdateType opens a ticker that schedules one update on its first tick.
type delayedUpdateType struct{ testEntityType }

// Open binds the fixture to the saved entity data.
func (delayedUpdateType) Open(_ *Tx, h *EntityHandle, d *EntityData) Entity {
	return &delayedUpdateEntity{testEntity: testEntity{handle: h, data: d}}
}

// delayedUpdateEntity schedules through the ordinary entity ticker contract.
type delayedUpdateEntity struct{ testEntity }

// Tick schedules once, then retains the entity until the update is delivered.
func (e *delayedUpdateEntity) Tick(tx *Tx, _ int64) {
	if e.data.Age == 0 {
		tx.ScheduleBlockUpdate(cube.Pos{0, 4, 0}, e.data.Data.(delayedUpdateBlock), time.Second/20)
	}
	e.data.Age += time.Second / 20
}

// delayedUpdateConfig stores the block to update on the first entity tick.
type delayedUpdateConfig struct{ block delayedUpdateBlock }

// Apply supplies the ticker's block without changing its position.
func (c delayedUpdateConfig) Apply(d *EntityData) { d.Data = c.block }

func TestEntityScheduledUpdateRetainsDelay(t *testing.T) {
	fired := false
	b := delayedUpdateBlock{fired: &fired}
	registry := NewBlockRegistry()
	registry.RegisterBlockState(BlockState{Name: "test:delayed_update", Properties: map[string]any{}})
	registry.RegisterBlock(b)
	w := Config{Synchronous: true, Blocks: registry}.New()
	defer w.Close()
	w.Do(func(tx *Tx) {
		tx.SetBlock(cube.Pos{0, 4, 0}, b, &SetOpts{DisableBlockUpdates: true, DisableRedstoneUpdates: true})
		tx.AddEntity(EntitySpawnOpts{}.New(delayedUpdateType{}, delayedUpdateConfig{block: b}))
	})
	w.AdvanceTick()
	if fired {
		t.Fatal("entity update executed in the tick that scheduled it")
	}
	w.AdvanceTick()
	if !fired {
		t.Fatal("entity update did not execute on the next tick")
	}
}

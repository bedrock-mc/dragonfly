package entity_test

import (
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/biome"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl64"
	"math"
	"testing"
)

// pastureGenerator builds a grass pasture without external carriers or fixtures.
type pastureGenerator struct{}

// GenerateChunk lays an open, naturally lit grass surface in plains.
func (pastureGenerator) GenerateChunk(_ world.ChunkPos, c *chunk.Chunk) {
	rid := world.DefaultBlockRegistry.BlockRuntimeID(block.Grass{})
	for x := range uint8(16) {
		for z := range uint8(16) {
			c.SetBlock(x, 0, z, 0, rid)
			for y := int16(c.Range().Min()); y <= int16(c.Range().Max()); y++ {
				c.SetBiome(x, y, z, uint32(biome.Plains{}.EncodeBiome()))
			}
		}
	}
}

// DefaultSpawn returns a healthy player position on the pasture.
func (pastureGenerator) DefaultSpawn(world.Dimension) cube.Pos { return cube.Pos{0, 1, 0} }

// animalPopulationObserver measures each admission's local population count.
type animalPopulationObserver struct {
	world.NopHandler
	maximum int
}

// HandleEntitySpawn observes the inclusive neighborhood after each natural admission.
func (h *animalPopulationObserver) HandleEntitySpawn(tx *world.Tx, e world.Entity) {
	a, ok := e.(*entity.Animal)
	if !ok {
		return
	}
	centre := world.ChunkPos{int32(math.Floor(a.Position()[0] / 16)), int32(math.Floor(a.Position()[2] / 16))}
	count := 0
	for e := range tx.Entities() {
		a, ok := e.(*entity.Animal)
		if !ok || a.Dead() {
			continue
		}
		pos := world.ChunkPos{int32(math.Floor(a.Position()[0] / 16)), int32(math.Floor(a.Position()[2] / 16))}
		if absChunkDelta(pos[0]-centre[0]) <= 4 && absChunkDelta(pos[1]-centre[1]) <= 4 {
			count++
		}
	}
	h.maximum = max(h.maximum, count)
}

// absChunkDelta returns the unsigned distance between resident column coordinates.
func absChunkDelta(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestNaturalPopulationRequiresPlayerAndRespectsCap(t *testing.T) {
	w := world.Config{Synchronous: true, Generator: pastureGenerator{}, Entities: entity.DefaultRegistry}.New()
	defer w.Close()
	w.SetTickRange(4)
	w.SetTime(6000)
	w.StopTime()
	observer := &animalPopulationObserver{}
	w.Handle(entity.NewAnimalSpawner(observer, 42))
	loader := world.NewLoader(4, w, world.NopViewer{})
	w.Do(func(tx *world.Tx) { loader.Move(tx, mgl64.Vec3{0, 1, 0}); loader.Load(tx, 100) })
	w.AdvanceTick()
	w.Do(func(tx *world.Tx) {
		for range tx.Entities() {
			t.Fatal("viewer without a player created passive actors")
		}
	})
	w.Do(func(tx *world.Tx) {
		tx.AddEntity(world.NewEntity(player.Type, player.Config{Position: mgl64.Vec3{0, 1, 0}, GameMode: world.GameModeCreative}))
	})
	for range 2000 {
		w.AdvanceTick()
	}
	w.Do(func(tx *world.Tx) {
		count := 0
		for e := range tx.Entities() {
			if _, ok := e.(*entity.Animal); !ok {
				continue
			}
			count++
			p := e.Position()
			feet := mgl64.Vec3{math.Floor(p[0]), 1, math.Floor(p[2])}
			// Distance is measured at integer feet before the horizontal spawn offset.
			if d := feet.Sub(mgl64.Vec3{0, 1, 0}).LenSqr(); d < 24*24 || d >= 44*44 {
				t.Errorf("actor outside admitted radius: %v", p)
			}
		}
		if count == 0 || observer.maximum > 4 {
			t.Fatalf("uncommanded count=%d, maximum local admission count=%d; want nonzero and local cap4", count, observer.maximum)
		}
		loader.Close(tx)
	})
}

func TestNaturalPopulationExcludesSpectatorsAndDarkHabitats(t *testing.T) {
	for _, mode := range []world.GameMode{world.GameModeSpectator, world.GameModeCreative} {
		w := world.Config{Synchronous: true, Generator: pastureGenerator{}, Entities: entity.DefaultRegistry}.New()
		w.SetTickRange(4)
		w.SetTime(18000)
		w.StopTime()
		w.Handle(entity.NewAnimalSpawner(w.Handler(), 42))
		loader := world.NewLoader(4, w, world.NopViewer{})
		w.Do(func(tx *world.Tx) {
			tx.AddEntity(world.NewEntity(player.Type, player.Config{Position: mgl64.Vec3{0, 1, 0}, GameMode: mode}))
			loader.Move(tx, mgl64.Vec3{0, 1, 0})
			loader.Load(tx, 100)
		})
		for range 1000 {
			w.AdvanceTick()
		}
		w.Do(func(tx *world.Tx) {
			for e := range tx.Entities() {
				if _, ok := e.(*entity.Animal); ok {
					t.Errorf("animal spawned with mode %v at night", mode)
				}
			}
			loader.Close(tx)
		})
		w.Close()
	}
}

func TestNaturalPopulationInSynchronousWorldWithoutLoaders(t *testing.T) {
	w := world.Config{Synchronous: true, Generator: pastureGenerator{}, Entities: entity.DefaultRegistry}.New()
	defer w.Close()
	w.SetTickRange(4)
	w.SetTime(6000)
	w.StopTime()
	w.Do(func(tx *world.Tx) {
		for x := -3; x <= 3; x++ {
			for z := -3; z <= 3; z++ {
				tx.Block(cube.Pos{x * 16, 1, z * 16})
			}
		}
	})
	// Complete first-generation initialization before enabling runtime population.
	w.AdvanceTick()
	observer := &animalPopulationObserver{}
	w.Handle(entity.NewAnimalSpawner(observer, 42))
	w.Do(func(tx *world.Tx) {
		tx.AddEntity(world.NewEntity(player.Type, player.Config{Position: mgl64.Vec3{0, 1, 0}, GameMode: world.GameModeCreative}))
	})
	for range 2000 {
		w.AdvanceTick()
	}
	w.Do(func(tx *world.Tx) {
		count := 0
		for e := range tx.Entities() {
			if _, ok := e.(*entity.Animal); ok {
				count++
			}
		}
		if count == 0 || observer.maximum > 4 {
			t.Fatalf("synchronous runtime count=%d, maximum local admission count=%d; want nonzero and local cap4", count, observer.maximum)
		}
	})
}

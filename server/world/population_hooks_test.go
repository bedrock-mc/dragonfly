package world

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"testing"
)

// populationObserver observes callbacks without relying on wall-clock ticking.
type populationObserver struct {
	NopHandler
	ticks     []int64
	generated []ChunkPos
}

// HandleTick records an owner callback and verifies chunk access is reentrant.
func (h *populationObserver) HandleTick(tx *Tx, tick int64) {
	h.ticks = append(h.ticks, tick)
	tx.Block(cube.Pos{0, 0, 0})
}

// HandleChunkGenerate records first-time generation on the world owner.
func (h *populationObserver) HandleChunkGenerate(tx *Tx, pos ChunkPos) {
	h.generated = append(h.generated, pos)
	tx.Block(cube.Pos{int(pos[0]) << 4, 0, int(pos[1]) << 4})
}

func TestPopulationHooksRunOnOwner(t *testing.T) {
	w := Config{Synchronous: true}.New()
	defer w.Close()
	h := &populationObserver{}
	w.Handle(h)
	w.Do(func(tx *Tx) { tx.Block(cube.Pos{0, 0, 0}) })
	w.AdvanceTick()
	w.AdvanceTick()
	if len(h.generated) != 1 || h.generated[0] != (ChunkPos{}) {
		t.Fatalf("generated callbacks = %v, want exactly the newly generated column", h.generated)
	}
	if len(h.ticks) != 2 || h.ticks[1] != h.ticks[0]+1 {
		t.Fatalf("tick callbacks = %v, want each advancing simulation tick", h.ticks)
	}
}

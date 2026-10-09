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

func TestPopulationHooksWithSharedSettings(t *testing.T) {
	provider := NopProvider{Set: defaultSettings()}
	first := Config{Synchronous: true, Provider: provider}.New()
	defer first.Close()
	second := Config{Synchronous: true, Provider: provider, Dim: Nether}.New()
	defer second.Close()
	h := &populationObserver{}
	second.Handle(h)
	second.Do(func(tx *Tx) { tx.Block(cube.Pos{0, 0, 0}) })
	first.AdvanceTick()
	second.AdvanceTick()
	if len(h.ticks) != 1 || len(h.generated) != 1 {
		t.Fatalf("shared-settings world skipped simulation callbacks: ticks=%v generated=%v", h.ticks, h.generated)
	}
}

// generatingTickObserver admits another column from the owner tick callback.
type generatingTickObserver struct{ populationObserver }

// HandleTick loads a fresh column after the initial generation snapshot.
func (h *generatingTickObserver) HandleTick(tx *Tx, tick int64) {
	h.populationObserver.HandleTick(tx, tick)
	tx.Block(cube.Pos{16, 0, 0})
}

func TestGenerationCallbackForColumnAdmittedByTickHandler(t *testing.T) {
	w := Config{Synchronous: true}.New()
	defer w.Close()
	h := &generatingTickObserver{}
	w.Handle(h)
	w.Do(func(tx *Tx) { tx.Block(cube.Pos{0, 0, 0}) })
	w.AdvanceTick()
	if len(h.generated) != 2 {
		t.Fatalf("tick-admitted column reached simulation without generation initialization: generated=%v", h.generated)
	}
}

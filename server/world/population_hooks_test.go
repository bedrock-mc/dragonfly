package world

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"math/rand/v2"
	"testing"
	"time"
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

// generationScheduledBlock observes initialization before scheduled simulation.
type generationScheduledBlock struct{ initialized, ticked *bool }

// ScheduledTick records the generation state observed by block simulation.
func (b generationScheduledBlock) ScheduledTick(cube.Pos, *Tx, *rand.Rand) {
	*b.ticked = *b.initialized
}

// EncodeBlock identifies the test block in its owning registry.
func (generationScheduledBlock) EncodeBlock() (string, map[string]any) {
	return "test:generation_scheduled", nil
}

// Hash returns the test block's unique registry key.
func (generationScheduledBlock) Hash() (uint64, uint64) { return 1 << 42, 0 }

// Model returns the test block's unused collision model.
func (generationScheduledBlock) Model() BlockModel { return nil }

// scheduledColumnGenerator supplies a scheduled block in a previously unloaded column.
type scheduledColumnGenerator struct {
	NopGenerator
	rid uint32
}

// GenerateChunk places the test block at the scheduled position.
func (g scheduledColumnGenerator) GenerateChunk(_ ChunkPos, c *chunk.Chunk) {
	c.SetBlock(0, 10, 0, 0, g.rid)
}

// scheduledGenerationObserver records initialization of the scheduled column.
type scheduledGenerationObserver struct {
	NopHandler
	initialized *bool
}

// HandleChunkGenerate marks the target column initialized before its block ticks.
func (h scheduledGenerationObserver) HandleChunkGenerate(_ *Tx, pos ChunkPos) {
	if pos == (ChunkPos{5, 5}) {
		*h.initialized = true
	}
}

func TestScheduledTickInitializesFreshColumn(t *testing.T) {
	initialized, ticked := false, false
	b := generationScheduledBlock{&initialized, &ticked}
	registry := NewBlockRegistry()
	registry.RegisterBlockState(BlockState{Name: "test:generation_scheduled", Properties: map[string]any{}})
	registry.RegisterBlock(b)
	registry.Finalize()
	w := Config{Synchronous: true, Blocks: registry, Generator: scheduledColumnGenerator{rid: registry.BlockRuntimeID(b)}}.New()
	defer w.Close()
	w.Handle(scheduledGenerationObserver{initialized: &initialized})
	w.Do(func(tx *Tx) { tx.ScheduleBlockUpdate(cube.Pos{80, 10, 80}, b, time.Second/20) })
	w.AdvanceTick()
	if !ticked {
		t.Fatal("scheduled block simulated before generation initialization")
	}
}

// generationSaveObserver tracks initialization, including columns admitted while closing.
type generationSaveObserver struct {
	NopHandler
	initialized map[ChunkPos]bool
}

// HandleChunkGenerate records completion of each column's one-time initialization.
func (h *generationSaveObserver) HandleChunkGenerate(_ *Tx, pos ChunkPos) { h.initialized[pos] = true }

// HandleClose admits a final column that must be initialized before saving.
func (h *generationSaveObserver) HandleClose(tx *Tx) { tx.Block(cube.Pos{16, 0, 0}) }

// generationSaveProvider observes whether initialization precedes persistence.
type generationSaveProvider struct {
	NopProvider
	observer *generationSaveObserver
	stored   map[ChunkPos]bool
}

// StoreColumn records initialization at the moment the column is persisted.
func (p *generationSaveProvider) StoreColumn(pos ChunkPos, _ Dimension, _ *chunk.Column) error {
	p.stored[pos] = p.observer.initialized[pos]
	return nil
}

func TestGenerationInitializationBeforePersistence(t *testing.T) {
	for _, mode := range []string{"save", "unload", "close"} {
		t.Run(mode, func(t *testing.T) {
			h := &generationSaveObserver{initialized: make(map[ChunkPos]bool)}
			p := &generationSaveProvider{observer: h, stored: make(map[ChunkPos]bool)}
			w := Config{Synchronous: true, Provider: p}.New()
			defer w.Close()
			w.Handle(h)
			w.Do(func(tx *Tx) { tx.Block(cube.Pos{}) })
			switch mode {
			case "save":
				w.Save()
			case "unload":
				w.Do(func(tx *Tx) { w.closeChunk(tx, ChunkPos{}, w.chunks[ChunkPos{}]) })
			case "close":
				w.Close()
			}
			if initialized, stored := p.stored[ChunkPos{}]; !stored || !initialized {
				t.Fatalf("column persisted before generation initialization: %v", p.stored)
			}
			if mode == "close" && !p.stored[ChunkPos{1, 0}] {
				t.Fatalf("close-admitted column skipped initialization: %v", p.stored)
			}
		})
	}
}

func TestSynchronousPopulationChunksWithoutLoaders(t *testing.T) {
	w := Config{Synchronous: true}.New()
	defer w.Close()
	w.SetTickRange(4)
	w.Do(func(tx *Tx) { tx.Block(cube.Pos{}); tx.Block(cube.Pos{16, 0, 0}) })
	w.AdvanceTick()
	w.Do(func(tx *Tx) {
		if len(tx.TickingChunks()) != 2 {
			t.Fatalf("synchronous population omitted resident columns: %v", tx.TickingChunks())
		}
		for _, pos := range []ChunkPos{{}, {1, 0}} {
			if tick, ok := tx.ChunkLastTick(pos); !ok || tick != tx.CurrentTick() {
				t.Fatalf("simulated column %v has no current tick stamp: %v %v", pos, tick, ok)
			}
		}
	})
}

// callbackScheduleObserver schedules one update through the selected owner callback.
type callbackScheduleObserver struct {
	NopHandler
	generation, scheduled bool
	block                 Block
}

// HandleTick requests a delayed update from the first simulation callback.
func (h *callbackScheduleObserver) HandleTick(tx *Tx, _ int64) {
	if !h.generation {
		h.schedule(tx)
	}
}

// HandleChunkGenerate requests a delayed update from the first generation callback.
func (h *callbackScheduleObserver) HandleChunkGenerate(tx *Tx, _ ChunkPos) {
	if h.generation {
		h.schedule(tx)
	}
}

// schedule requests exactly one tick of delay from the callback's current tick.
func (h *callbackScheduleObserver) schedule(tx *Tx) {
	if !h.scheduled {
		h.scheduled = true
		tx.ScheduleBlockUpdate(cube.Pos{0, 10, 0}, h.block, time.Second/20)
	}
}

func TestOwnerCallbacksPreserveScheduledDelay(t *testing.T) {
	for _, generation := range []bool{false, true} {
		t.Run(map[bool]string{false: "tick", true: "generation"}[generation], func(t *testing.T) {
			initialized, ticked := true, false
			b := generationScheduledBlock{&initialized, &ticked}
			registry := NewBlockRegistry()
			registry.RegisterBlockState(BlockState{Name: "test:generation_scheduled", Properties: map[string]any{}})
			registry.RegisterBlock(b)
			w := Config{Synchronous: true, Blocks: registry}.New()
			defer w.Close()
			w.Handle(&callbackScheduleObserver{generation: generation, block: b})
			w.Do(func(tx *Tx) {
				tx.SetBlock(cube.Pos{0, 10, 0}, b, &SetOpts{DisableBlockUpdates: true, DisableRedstoneUpdates: true})
			})
			w.AdvanceTick()
			if ticked {
				t.Fatal("one-tick callback delay executed immediately")
			}
			w.AdvanceTick()
			if !ticked {
				t.Fatal("one-tick callback delay did not execute on the next tick")
			}
		})
	}
}

// chainedGenerationObserver admits a finite neighbor chain without nested callbacks.
type chainedGenerationObserver struct {
	NopHandler
	seen           map[ChunkPos]int
	active, nested bool
}

// HandleChunkGenerate exercises reentrant terrain reads and settings access.
func (h *chainedGenerationObserver) HandleChunkGenerate(tx *Tx, pos ChunkPos) {
	if h.active {
		h.nested = true
	}
	h.active = true
	defer func() { h.active = false }()
	h.seen[pos]++
	tx.World().Time()
	tx.Block(cube.Pos{int(pos[0]) * 16, 0, 0})
	if pos[0] < 2 {
		tx.Block(cube.Pos{int(pos[0]+1) * 16, 0, 0})
	}
}

func TestGenerationAdmissionChainUsesOwnerWithoutRecursion(t *testing.T) {
	w := Config{Synchronous: true}.New()
	defer w.Close()
	h := &chainedGenerationObserver{seen: make(map[ChunkPos]int)}
	w.Handle(h)
	w.Do(func(tx *Tx) { tx.Block(cube.Pos{}) })
	if h.nested || len(h.seen) != 3 {
		t.Fatalf("generation chain: nested=%v callbacks=%v", h.nested, h.seen)
	}
	w.AdvanceTick()
	w.Save()
	for pos, calls := range h.seen {
		if calls != 1 {
			t.Fatalf("column %v initialized %d times", pos, calls)
		}
	}
}

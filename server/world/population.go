package world

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"slices"
)

// compareChunkPos orders columns consistently for owner callbacks.
func compareChunkPos(a, b ChunkPos) int {
	if a[0] < b[0] {
		return -1
	}
	if a[0] > b[0] {
		return 1
	}
	if a[1] < b[1] {
		return -1
	}
	if a[1] > b[1] {
		return 1
	}
	return 0
}

// TickingChunks returns loaded columns within the world's simulation distance
// of a loader, or all resident columns in synchronous worlds. The result is a
// snapshot and querying it never generates terrain.
func (tx *Tx) TickingChunks() []ChunkPos {
	tx.rejectDetached()
	w := tx.World()
	_, loaders := w.allViewers()
	centres := make([]ChunkPos, 0, len(loaders))
	for _, l := range loaders {
		l.mu.RLock()
		centres = append(centres, l.pos)
		l.mu.RUnlock()
	}
	result := make([]ChunkPos, 0)
	for pos := range w.chunks {
		if w.conf.Synchronous || (ticker{}).anyWithinDistance(pos, centres, int32(w.tickRange())) {
			result = append(result, pos)
		}
	}
	slices.SortFunc(result, compareChunkPos)
	return result
}

// ChunkLoaded reports whether a column is resident without loading it.
func (tx *Tx) ChunkLoaded(pos ChunkPos) bool {
	tx.rejectDetached()
	_, ok := tx.World().chunks[pos]
	return ok
}

// LightLevels returns unattenuated sky and block light for spawn admission.
func (tx *Tx) LightLevels(pos cube.Pos) (sky, block uint8) {
	return tx.SkyLight(pos), tx.BlockLight(pos)
}

// TickRange returns the simulation distance in columns.
func (w *World) TickRange() int { return w.tickRange() }

// ChunkLastTick returns the most recent simulation tick of a resident column.
// It never loads a column; absent or inactive neighbours cannot enable spawning.
func (tx *Tx) ChunkLastTick(pos ChunkPos) (int64, bool) {
	tx.rejectDetached()
	c, ok := tx.World().chunks[pos]
	if !ok {
		return 0, false
	}
	return c.lastTick, c.lastTick > 0
}

// readChunk completes generation initialization before exposing terrain to an
// ordinary owner read. Reads inside generation callbacks see admitted terrain;
// their newly admitted columns are initialized after that callback returns.
func (tx *Tx) readChunk(pos ChunkPos) *Column {
	c := tx.chunk(pos)
	tx.finishChunkAdmission(pos, c)
	return c
}

// finishChunkAdmission completes pending initialization outside generation callbacks.
func (tx *Tx) finishChunkAdmission(pos ChunkPos, c *Column) {
	if c == nil {
		return
	}
	pending := c.generated && !tx.generating
	tx.initializeColumn(pos, c)
	if pending {
		(ticker{}).dispatchGeneration(tx)
	}
}

// initializeColumn invokes the optional handler once, on the same owner
// transaction and without holding settings or entity locks.
func (tx *Tx) initializeColumn(pos ChunkPos, c *Column) {
	if !c.generated || tx.generating {
		return
	}
	c.generated = false
	tx.generating = true
	defer func() { tx.generating = false }()
	if h, ok := tx.World().Handler().(GenerationHandler); ok {
		h.HandleChunkGenerate(tx, pos)
	}
}

// deliverColumn initializes terrain before handing it to a loader callback.
// Delivery requested within generation is deferred until that callback returns.
func (tx *Tx) deliverColumn(pos ChunkPos, c *Column, callback chunkCallback) {
	if c != nil && tx.generating {
		tx.Defer(func(next *Tx) {
			if current := next.World().chunks[pos]; current != c {
				callback(next, nil)
				return
			}
			next.deliverColumn(pos, c, callback)
		})
		return
	}
	tx.finishChunkAdmission(pos, c)
	callback(tx, c)
}

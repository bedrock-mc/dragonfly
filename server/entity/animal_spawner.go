package entity

import (
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

const (
	animalSurfaceCap             = 4
	populationNeighbourhood      = 4
	naturalPopulationLimit       = 200
	minimumSpawnDistance         = 24
	maximumSpawnDistance         = 128
	smallSimulationSpawnDistance = 44
	spawnHorizontalOffset        = float64(float32(.49))
)

// AnimalSpawner populates eligible surface habitats with passive world actors.
// It is driven exclusively by world-owner tick and generation callbacks. A
// spawner must be attached to only one world and must not be called concurrently.
type AnimalSpawner struct {
	world.Handler
	random  *rand.Rand
	players []mgl64.Vec3
}

// NewAnimalSpawner composes passive population with the world's existing handler.
// Attach it before loading chunks. The seed controls scheduling randomness, not
// terrain generation; no timer or background population goroutine is started.
func NewAnimalSpawner(previous world.Handler, seed uint64) *AnimalSpawner {
	if previous == nil {
		previous = world.NopHandler{}
	}
	return &AnimalSpawner{Handler: previous, random: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// HandleChunkGenerate attempts the new column's population exactly once.
func (s *AnimalSpawner) HandleChunkGenerate(tx *world.Tx, pos world.ChunkPos) {
	if h, ok := s.Handler.(world.GenerationHandler); ok {
		h.HandleChunkGenerate(tx, pos)
	}
	if tx.Dimension() != world.Overworld {
		return
	}
	s.collectPlayers(tx)
	if len(s.players) > 0 {
		s.attempt(tx, pos)
	}
}

// HandleTick schedules surface population on active, already loaded columns.
func (s *AnimalSpawner) HandleTick(tx *world.Tx, tick int64) {
	if h, ok := s.Handler.(world.TickHandler); ok {
		h.HandleTick(tx, tick)
	}
	if tx.Dimension() != world.Overworld {
		return
	}
	s.collectPlayers(tx)
	if len(s.players) == 0 {
		return
	}
	for _, pos := range tx.TickingChunks() {
		if s.random.IntN(2000) <= 10 {
			s.attempt(tx, pos)
		}
	}
}

// collectPlayers snapshots living, interactable player positions on the owner.
func (s *AnimalSpawner) collectPlayers(tx *world.Tx) {
	s.players = s.players[:0]
	for e := range tx.Entities() {
		p, ok := e.(interface {
			GameMode() world.GameMode
			Dead() bool
		})
		if ok && !p.Dead() && p.GameMode().AllowsInteraction() {
			s.players = append(s.players, e.Position())
		}
	}
}

// attempt selects one habitat rule before checking supporting blocks and placing
// its herd. Category counts are rechecked for each successful world admission.
func (s *AnimalSpawner) attempt(tx *world.Tx, column world.ChunkPos) {
	x := int(column[0])<<4 + s.random.IntN(16)
	z := int(column[1])<<4 + s.random.IntN(16)
	floor, ok := animalSurface(tx, x, z)
	if !ok {
		return
	}
	feet := floor.Side(cube.FaceUp)
	rules := eligibleAnimalRules(tx, feet)
	weight := 0
	for _, rule := range rules {
		weight += rule.weight
	}
	if weight == 0 {
		return
	}
	choice := s.random.IntN(weight)
	selected := rules[0]
	for _, rule := range rules {
		choice -= rule.weight
		if choice < 0 {
			selected = rule
			break
		}
	}
	u := s.random.Float64()
	herd := selected.minimum + int(math.Round(u*u*float64(selected.maximum-selected.minimum)))
	if !s.admitted(tx, feet, selected.species) {
		return
	}
	for range herd {
		local, global := animalPopulation(tx, column)
		if local >= animalSurfaceCap || global >= naturalPopulationLimit {
			return
		}
		opts := world.EntitySpawnOpts{Position: mgl64.Vec3{
			float64(feet[0]) + spawnHorizontalOffset, float64(feet[1]), float64(feet[2]) + spawnHorizontalOffset,
		}}
		tx.AddEntity(opts.New(selected.species, naturalAnimalConfig{species: selected.species, baby: s.random.IntN(100) < 5}))
	}
}

// admitted applies player-distance, loaded-neighbour, grass and block-volume gates.
func (s *AnimalSpawner) admitted(tx *world.Tx, feet cube.Pos, species animalType) bool {
	at := mgl64.Vec3{float64(feet[0]), float64(feet[1]), float64(feet[2])}
	nearest := math.Inf(1)
	for _, p := range s.players {
		nearest = min(nearest, p.Sub(at).LenSqr())
	}
	maximum := maximumSpawnDistance
	distance := tx.World().TickRange()
	if distance <= 0 {
		return false
	}
	if distance == 4 {
		maximum = smallSimulationSpawnDistance
	}
	if nearest < minimumSpawnDistance*minimumSpawnDistance || nearest >= float64(maximum*maximum) {
		return false
	}
	if distance > 4 {
		centre := world.ChunkPos{int32(feet[0] >> 4), int32(feet[2] >> 4)}
		for dx := int32(-1); dx <= 1; dx++ {
			for dz := int32(-1); dz <= 1; dz++ {
				last, ok := tx.ChunkLastTick(world.ChunkPos{centre[0] + dx, centre[1] + dz})
				if !ok || tx.CurrentTick()-last > 1 {
					return false
				}
			}
		}
	}
	name, _ := tx.Block(feet.Side(cube.FaceDown)).EncodeBlock()
	if name != "minecraft:grass" && name != "minecraft:grass_block" {
		return false
	}
	return animalVolumeClear(tx, feet, species)
}

// animalPopulation counts surface-category actors in the surrounding 9x9
// columns and living non-player actors toward the world's global limit.
func animalPopulation(tx *world.Tx, centre world.ChunkPos) (local, global int) {
	for e := range tx.Entities() {
		if _, player := e.(interface{ GameMode() world.GameMode }); player {
			continue
		}
		if l, ok := e.(Living); ok && !l.Dead() {
			global++
		}
		a, ok := e.(*Animal)
		if !ok || a.Dead() || !a.state().surface {
			continue
		}
		p := a.Position()
		x, z := int32(math.Floor(p[0]))>>4, int32(math.Floor(p[2]))>>4
		if abs32(x-centre[0]) <= populationNeighbourhood && abs32(z-centre[1]) <= populationNeighbourhood {
			local++
		}
	}
	return
}

// abs32 returns the absolute chunk displacement.
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

type naturalAnimalConfig struct {
	species animalType
	baby    bool
}

// Apply marks successful population as surface-natural for future cap accounting.
func (c naturalAnimalConfig) Apply(data *world.EntityData) {
	c.species.Apply(data)
	b := data.Data.(*animalState)
	b.surface, b.natural = true, true
	if c.baby {
		b.babyUntil = time.Minute * 20
	}
}

type animalRule struct {
	species                  animalType
	weight, minimum, maximum int
	tags                     []string
}

var animalRules = []animalRule{
	{species: CowType, weight: 8, minimum: 2, maximum: 3, tags: []string{"animal"}},
	{species: PigType, weight: 10, minimum: 1, maximum: 3, tags: []string{"animal", "cherry_grove"}},
	{species: SheepType, weight: 12, minimum: 2, maximum: 3, tags: []string{"animal"}},
	{species: SheepType, weight: 2, minimum: 2, maximum: 4, tags: []string{"meadow", "cherry_grove"}},
	{species: ChickenType, weight: 10, minimum: 2, maximum: 4, tags: []string{"animal"}},
}

// eligibleAnimalRules evaluates biome and inclusive brightness before selection.
func eligibleAnimalRules(tx *world.Tx, feet cube.Pos) []animalRule {
	result := make([]animalRule, 0, len(animalRules))
	if animalBrightness(tx, feet) < 7 {
		return result
	}
	biome := tx.Biome(feet)
	if biome == nil {
		return result
	}
	tags := biome.Tags()
	for _, rule := range animalRules {
		for _, tag := range rule.tags {
			if slices.Contains(tags, tag) {
				result = append(result, rule)
				break
			}
		}
	}
	return result
}

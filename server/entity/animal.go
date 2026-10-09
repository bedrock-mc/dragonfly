package entity

import (
	"math"
	"time"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/internal/nbtconv"
	"github.com/df-mc/dragonfly/server/world"
)

// CowType is the persistent type of a cow.
var CowType = animalType{name: "minecraft:cow", width: .9, height: 1.3, health: 10}

// PigType is the persistent type of a pig.
var PigType = animalType{name: "minecraft:pig", width: .9, height: .9, health: 10}

// SheepType is the persistent type of a sheep.
var SheepType = animalType{name: "minecraft:sheep", width: .9, height: 1.3, health: 8}

// ChickenType is the persistent type of a chicken.
var ChickenType = animalType{name: "minecraft:chicken", width: .6, height: .8, health: 4}

// NewCow creates an adult cow that may be added to a world.
func NewCow(opts world.EntitySpawnOpts) *world.EntityHandle { return opts.New(CowType, CowType) }

// NewPig creates an adult pig that may be added to a world.
func NewPig(opts world.EntitySpawnOpts) *world.EntityHandle { return opts.New(PigType, PigType) }

// NewSheep creates an adult sheep that may be added to a world.
func NewSheep(opts world.EntitySpawnOpts) *world.EntityHandle { return opts.New(SheepType, SheepType) }

// NewChicken creates an adult chicken that may be added to a world.
func NewChicken(opts world.EntitySpawnOpts) *world.EntityHandle {
	return opts.New(ChickenType, ChickenType)
}

type animalType struct {
	name                  string
	width, height, health float64
}

// EncodeEntity returns the species identifier used in saves and actor packets.
func (t animalType) EncodeEntity() string { return t.name }

// BBox returns the adult species collision bounds in feet space.
func (t animalType) BBox(e world.Entity) cube.BBox {
	scale := 1.0
	if a, ok := e.(*Animal); ok {
		scale = a.Scale()
	}
	half := t.width * scale / 2
	return cube.Box(-half, 0, -half, half, t.height*scale, half)
}

// Open exposes persistent state through a transaction-scoped living entity.
func (t animalType) Open(tx *world.Tx, h *world.EntityHandle, data *world.EntityData) world.Entity {
	return &Animal{Ent: Open(tx, h, data)}
}

// Apply initialises a healthy adult animal with environmental movement.
func (t animalType) Apply(data *world.EntityData) {
	data.Data = &animalState{
		BaseBehaviour: NewBaseBehaviour(),
		health:        NewHealthManager(t.health, t.health),
		effects:       NewEffectManager(),
		movement:      MovementComputer{Gravity: .08, Drag: .02},
		speed:         .25,
	}
}

// DecodeNBT restores mutable living state without replacing shared actor fields.
func (t animalType) DecodeNBT(m map[string]any, data *world.EntityData) {
	t.Apply(data)
	b := data.Data.(*animalState)
	if v, ok := m["Health"]; ok {
		health := float64(nbtconv.Float32(map[string]any{"Health": v}, "Health"))
		maximum := float64(nbtconv.Float32(m, "MaxHealth"))
		if maximum <= 0 || math.IsNaN(maximum) || math.IsInf(maximum, 0) {
			maximum = t.health
		}
		if math.IsNaN(health) || math.IsInf(health, 0) {
			health = maximum
		}
		b.health = NewHealthManager(max(health, 0), maximum)
	}
	if speed := nbtconv.Float64(m, "MovementSpeed"); speed > 0 && !math.IsInf(speed, 0) {
		b.speed = speed
	}
	b.babyUntil = time.Duration(nbtconv.Int64(m, "BabyUntil"))
	b.surface, b.natural = nbtconv.Bool(m, "Surface"), nbtconv.Bool(m, "NaturalSpawn")
	b.deathAge = time.Duration(nbtconv.Int64(m, "DeathAge"))
}

// EncodeNBT serialises living state alongside the world's position and UUID data.
func (t animalType) EncodeNBT(data *world.EntityData) map[string]any {
	b := data.Data.(*animalState)
	return map[string]any{
		"BabyUntil": int64(b.babyUntil),
		"Surface":   boolByte(b.surface), "NaturalSpawn": boolByte(b.natural),
		"Health": float32(b.health.Health()), "MaxHealth": float32(b.health.MaxHealth()),
		"MovementSpeed": b.speed, "DeathAge": int64(b.deathAge),
	}
}

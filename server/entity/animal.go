package entity

import (
	"github.com/df-mc/dragonfly/server/entity/effect"
	"math"
	"reflect"
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
	switch a := e.(type) {
	case *Animal:
		scale = a.Scale()
	case *Ent:
		if b, ok := a.data.Data.(*animalState); ok && a.Age() < b.babyUntil {
			scale = .5
		}
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
	if age, ok := m["AnimalAge"].(int64); ok && age >= 0 {
		data.Age = time.Duration(age)
	}
	restoreAnimalEffects(m, b)
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
	if speed, ok := m["MovementSpeed"].(float64); ok && speed >= 0 && !math.IsNaN(speed) && !math.IsInf(speed, 0) {
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
		"AnimalAge": int64(data.Age), "Effects": encodeAnimalEffects(b),
		"Surface": boolByte(b.surface), "NaturalSpawn": boolByte(b.natural),
		"Health": float32(b.health.Health()), "MaxHealth": float32(b.health.MaxHealth()),
		"MovementSpeed": b.speed, "DeathAge": int64(b.deathAge),
	}
}

// encodeAnimalEffects preserves lasting modifiers and their remaining durations.
func encodeAnimalEffects(b *animalState) []map[string]any {
	entries := make([]map[string]any, 0, len(b.effects.Effects()))
	for _, e := range b.effects.Effects() {
		id, ok := effect.ID(e.Type())
		if !ok {
			continue
		}
		entries = append(entries, map[string]any{"ID": int32(id), "Level": int32(e.Level()), "Duration": int64(e.Duration()), "Ambient": boolByte(e.Ambient()), "Infinite": boolByte(e.Infinite()), "Hidden": boolByte(e.ParticlesHidden())})
	}
	return entries
}

// restoreAnimalEffects restores already-applied modifiers without starting them twice.
func restoreAnimalEffects(m map[string]any, b *animalState) {
	for _, entry := range nbtconv.Slice(m, "Effects") {
		d, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		typ, ok := effect.ByID(int(nbtconv.Int32(d, "ID")))
		if !ok {
			continue
		}
		lasting, ok := typ.(effect.LastingType)
		if !ok {
			continue
		}
		level := int(nbtconv.Int32(d, "Level"))
		duration := max(time.Duration(nbtconv.Int64(d, "Duration")), 0)
		if level <= 0 {
			continue
		}
		e := effect.New(lasting, level, duration)
		if nbtconv.Bool(d, "Ambient") {
			e = effect.NewAmbient(lasting, level, duration)
		}
		if nbtconv.Bool(d, "Infinite") {
			e = effect.NewInfinite(lasting, level)
		}
		if nbtconv.Bool(d, "Hidden") {
			e = e.WithoutParticles()
		}
		b.effects.effects[reflect.TypeOf(typ)] = e
	}
}

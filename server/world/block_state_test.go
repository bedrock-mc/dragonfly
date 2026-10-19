package world

import (
	"math"
	"testing"
)

func TestUnknownBlockContainerSize(t *testing.T) {
	tests := map[string]struct {
		name string
		data map[string]any
		want int
	}{
		"single trapped chest": {
			name: "minecraft:trapped_chest",
			want: 27,
		},
		"paired trapped chest": {
			name: "minecraft:trapped_chest",
			data: map[string]any{"pairx": int32(2), "pairz": int32(3)},
			want: 54,
		},
		"single copper chest": {
			name: "minecraft:copper_chest",
			want: 27,
		},
		"paired waxed weathered copper chest": {
			name: "minecraft:waxed_weathered_copper_chest",
			data: map[string]any{"pairx": int32(2), "pairz": int32(3)},
			want: 54,
		},
		"unrelated unknown block": {
			name: "minecraft:copper_chestplate",
			want: 0,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := unknownBlock{BlockState: BlockState{Name: test.name}, data: test.data}
			if got := b.ContainerSize(); got != test.want {
				t.Fatalf("container size = %d, want %d", got, test.want)
			}
		})
	}
}

// TestUnknownBlockSharedProperties checks native scalar fallback values.
func TestUnknownBlockSharedProperties(t *testing.T) {
	for _, test := range []struct {
		name                string
		friction            float64
		dampening, emission uint8
	}{
		{"minecraft:air", .899999, 0, 0},
		{"minecraft:stone", .6, 15, 0},
		{"minecraft:glass", .6, 0, 0},
		{"minecraft:ice", .98, 3, 0},
		{"minecraft:water", .6, 1, 0},
		{"minecraft:lit_redstone_lamp", .6, 15, 15},
		{"minecraft:campfire", .6, 0, 0},
		{"minecraft:soul_campfire", .6, 0, 0},
		{"minecraft:sea_pickle", .6, 0, 0},
		{"test:unlisted_block", .6, 15, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := unknownBlock{BlockState: BlockState{Name: test.name}}
			// The shared source stores friction as float32; preserve the value
			// within that precision without changing the fallback policy.
			if got := b.Friction(); math.Abs(got-test.friction) > 1e-6 {
				t.Errorf("friction = %v, want %v", got, test.friction)
			}
			if got := b.LightDiffusionLevel(); got != test.dampening {
				t.Errorf("light dampening = %v, want %v", got, test.dampening)
			}
			if got := b.LightEmissionLevel(); got != test.emission {
				t.Errorf("light emission = %v, want %v", got, test.emission)
			}
		})
	}
}

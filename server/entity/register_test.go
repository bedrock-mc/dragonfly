package entity

import "testing"

func TestDefaultRegistryExcludesPassiveAnimals(t *testing.T) {
	for _, name := range []string{"minecraft:cow", "minecraft:pig", "minecraft:sheep", "minecraft:chicken"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := DefaultRegistry.Lookup(name); ok {
				t.Fatalf("default registry implements experimental animal %s", name)
			}
		})
	}
	for _, name := range []string{"minecraft:item", "minecraft:tnt", "minecraft:arrow"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := DefaultRegistry.Lookup(name); !ok {
				t.Fatalf("default registry lost supported entity %s", name)
			}
		})
	}
}

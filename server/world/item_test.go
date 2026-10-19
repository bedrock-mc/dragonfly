package world

import (
	"testing"

	vanilla "github.com/cinnabar-mc/protocolgen/generated/data/item"
)

func TestVanillaItemEntries(t *testing.T) {
	entries := VanillaItemEntries()
	if len(entries) < 1000 {
		t.Fatalf("expected vanilla item dictionary to contain many entries, got %d", len(entries))
	}
	entry, ok := entries["minecraft:mace"]
	if !ok {
		t.Fatal("expected vanilla item dictionary to contain minecraft:mace")
	}
	if entry.RuntimeID == 0 {
		t.Fatal("expected minecraft:mace to have a runtime ID")
	}

	entries["minecraft:mace"] = VanillaItemEntry{}
	entry, ok = VanillaItemEntryByName("minecraft:mace")
	if !ok {
		t.Fatal("expected minecraft:mace lookup to succeed")
	}
	if entry.RuntimeID == 0 {
		t.Fatal("expected copied vanilla item map mutation not to affect package state")
	}
}

func TestVanillaItemEntryByNameCopiesData(t *testing.T) {
	entry, ok := VanillaItemEntryByName("minecraft:diamond_sword")
	if !ok {
		t.Fatal("expected minecraft:diamond_sword lookup to succeed")
	}
	if len(entry.Data) == 0 {
		t.Skip("minecraft:diamond_sword has no data map to verify copy behaviour")
	}
	for key := range entry.Data {
		entry.Data[key] = "mutated"
		next, ok := VanillaItemEntryByName("minecraft:diamond_sword")
		if !ok {
			t.Fatal("expected minecraft:diamond_sword lookup to succeed")
		}
		if next.Data[key] == "mutated" {
			t.Fatal("expected vanilla item entry data mutation not to affect package state")
		}
		return
	}
}

// TestVanillaItemEntriesHaveSharedStackSizes checks native catalog stack limits.
func TestVanillaItemEntriesHaveSharedStackSizes(t *testing.T) {
	for name, entry := range VanillaItemEntries() {
		definition, ok := vanilla.LookupRuntime(name)
		if !ok {
			t.Errorf("vanilla item %s has no shared runtime definition", name)
			continue
		}
		if definition.MaxCount <= 0 || entry.MaxStackSize != definition.MaxCount {
			t.Errorf("%s stack size = %d, want shared positive size %d", name, entry.MaxStackSize, definition.MaxCount)
		}
	}
}

// TestVanillaItemStackSizes checks representative stack categories.
func TestVanillaItemStackSizes(t *testing.T) {
	for name, want := range map[string]int{
		"minecraft:apple":         64,
		"minecraft:bucket":        16,
		"minecraft:ender_pearl":   16,
		"minecraft:diamond_sword": 1,
		"minecraft:acacia_boat":   1,
		"minecraft:copper_spear":  1,
	} {
		entry, ok := VanillaItemEntryByName(name)
		if !ok || entry.MaxStackSize != want {
			t.Errorf("%s stack size = %d (present %t), want %d", name, entry.MaxStackSize, ok, want)
		}
	}
}

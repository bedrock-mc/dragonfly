package recipe

import (
	"slices"
	"testing"

	vanilla "github.com/cinnabar-mc/protocolgen/generated/data/item"
)

func TestItemsInTag(t *testing.T) {
	items := ItemsInTag("minecraft:is_sword")
	if !slices.Contains(items, "minecraft:mace") {
		t.Fatal("expected minecraft:is_sword to contain minecraft:mace")
	}

	items[0] = "minecraft:mutated"
	if NewItemTag("minecraft:is_sword", 1).Contains("minecraft:mutated") {
		t.Fatal("expected returned tag item slice mutation not to affect package state")
	}
}

func TestItemTagsReturnsCopy(t *testing.T) {
	tags := ItemTags()
	items, ok := tags["minecraft:is_sword"]
	if !ok {
		t.Fatal("expected minecraft:is_sword tag")
	}
	if !slices.Contains(items, "minecraft:mace") {
		t.Fatal("expected minecraft:is_sword to contain minecraft:mace")
	}

	tags["minecraft:is_sword"] = nil
	items[0] = "minecraft:mutated"
	if !NewItemTag("minecraft:is_sword", 1).Contains("minecraft:mace") {
		t.Fatal("expected returned tag map and slice mutation not to affect package state")
	}
}

// TestItemTagsMatchSharedCatalog checks every native tag membership exposed to recipes.
func TestItemTagsMatchSharedCatalog(t *testing.T) {
	wantMemberships := 0
	for _, item := range vanilla.RuntimeItems() {
		for _, tag := range vanilla.RuntimeTags(item.Name) {
			wantMemberships++
			if !NewItemTag(tag, 1).Contains(item.Name) {
				t.Errorf("tag %s is missing catalog item %s", tag, item.Name)
			}
		}
	}

	gotMemberships := 0
	for tag, names := range ItemTags() {
		if !slices.IsSorted(names) {
			t.Errorf("tag %s members are not in name order", tag)
		}
		for _, name := range names {
			gotMemberships++
			if !slices.Contains(vanilla.RuntimeTags(name), tag) {
				t.Errorf("tag %s contains item %s without catalog membership", tag, name)
			}
		}
	}
	if gotMemberships != wantMemberships {
		t.Errorf("tag membership count = %d, want %d", gotMemberships, wantMemberships)
	}
}

// TestUnknownItemTag preserves empty lookup and clamped count behavior.
func TestUnknownItemTag(t *testing.T) {
	const unknown = "dragonfly:unknown"
	if got := ItemsInTag(unknown); got != nil {
		t.Fatalf("unknown tag items = %v, want nil", got)
	}
	if _, ok := ItemTags()[unknown]; ok {
		t.Fatal("unknown tag appeared in the catalog")
	}
	tag := NewItemTag(unknown, -1)
	if tag.Count() != 0 || !tag.Empty() || tag.Tag() != unknown || tag.Contains("minecraft:air") {
		t.Fatalf("unexpected unknown tag behavior: %+v", tag)
	}
}

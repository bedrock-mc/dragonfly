package recipe_test

import (
	"testing"
	_ "unsafe"

	_ "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/potion"
	"github.com/df-mc/dragonfly/server/item/recipe"
	"github.com/df-mc/dragonfly/server/world"
)

//go:linkname registerVanilla github.com/df-mc/dragonfly/server/item/recipe.registerVanilla
func registerVanilla()

// TestTrialPotionsBrewFromAwkward checks that the 1.21 potions brew to their own types rather
// than falling back to water: an unknown output type used to resolve to potion 0.
func TestTrialPotionsBrewFromAwkward(t *testing.T) {
	world.DefaultBlockRegistry.Finalize()
	registerVanilla()

	reagents := map[string]potion.Potion{
		"minecraft:slime":      potion.Oozing(),
		"minecraft:stone":      potion.Infested(),
		"minecraft:web":        potion.Weaving(),
		"minecraft:breeze_rod": potion.WindCharged(),
	}
	found := map[string]bool{}
	for _, r := range recipe.Recipes() {
		p, ok := r.(recipe.Potion)
		if !ok {
			continue
		}
		input, ok := p.Input()[0].(item.Stack).Item().(item.Potion)
		if !ok || input.Type != potion.Awkward() {
			continue
		}
		name, _ := p.Input()[1].(item.Stack).Item().EncodeItem()
		want, ok := reagents[name]
		if !ok {
			continue
		}
		output, ok := p.Output()[0].Item().(item.Potion)
		if !ok {
			t.Fatalf("awkward + %s: output %T is not a drinkable potion", name, p.Output()[0].Item())
		}
		if output.Type != want {
			t.Errorf("awkward + %s: got potion %d, want %d", name, output.Type.Uint8(), want.Uint8())
		}
		found[name] = true
	}
	for name := range reagents {
		if !found[name] {
			t.Errorf("no awkward + %s recipe", name)
		}
	}
	if effects := potion.Oozing().Effects(); len(effects) != 1 {
		t.Errorf("oozing has %d effects, want 1", len(effects))
	}
}

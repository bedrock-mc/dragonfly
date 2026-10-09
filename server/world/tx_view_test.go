package world

import (
	"reflect"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
)

func TestRunBlockTransaction_Reliability(t *testing.T) {
	t.Parallel()

	view := &blockTransactionTestView{blocks: map[cube.Pos]Block{}}
	if !RunBlockTransaction(view, func(tx *Tx) {
		tx.SetBlock(cube.Pos{}, nil, nil)
		if _, ok := tx.BlockLoaded(cube.Pos{}); !ok {
			t.Fatal("detached write was not readable")
		}
	}) {
		t.Fatal("supported detached transaction was marked incomplete")
	}
	if RunBlockTransaction(view, func(tx *Tx) { _ = tx.Light(cube.Pos{}) }) {
		t.Fatal("unsupported detached light query was reported complete")
	}
	if RunBlockTransaction(view, func(tx *Tx) { _ = tx.World() }) {
		t.Fatal("unsupported detached world access was reported complete")
	}
	if RunBlockTransaction(view, func(tx *Tx) {
		func() {
			defer func() { _ = recover() }()
			_ = tx.World()
		}()
	}) {
		t.Fatal("recovered unsupported access was reported complete")
	}
	if RunBlockTransaction(view, func(tx *Tx) { _ = tx.RedstonePower(cube.Pos{}) }) {
		t.Fatal("unsupported detached redstone query was reported complete")
	}
	if RunBlockTransaction(view, func(tx *Tx) { tx.SetBlockEntity(cube.Pos{}, nil) }) {
		t.Fatal("unsupported detached block-entity write was reported complete")
	}
	if !RunBlockTransaction(view, func(tx *Tx) {
		if got := tx.Dimension(); got != Overworld {
			t.Fatalf("dimension = %v, want Overworld", got)
		}
	}) {
		t.Fatal("supported detached dimension query was marked incomplete")
	}
	if RunBlockTransaction(nil, func(*Tx) {}) || RunBlockTransaction(view, nil) {
		t.Fatal("nil detached transaction input was reported complete")
	}
}

func TestRunBlockTransaction_PreservesCallbackPanics(t *testing.T) {
	t.Parallel()

	const want = "callback panic"
	defer func() {
		if got := recover(); got != want {
			t.Fatalf("recovered panic = %v, want %q", got, want)
		}
	}()
	RunBlockTransaction(&blockTransactionTestView{}, func(*Tx) { panic(want) })
}

func TestRunBlockTransaction_ClosesDetachedTransaction(t *testing.T) {
	t.Parallel()

	var escaped *Tx
	if !RunBlockTransaction(&blockTransactionTestView{}, func(tx *Tx) { escaped = tx }) {
		t.Fatal("empty detached transaction was marked incomplete")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("detached transaction remained usable after its callback")
		}
	}()
	_ = escaped.Range()
}

func TestBlockTransactionView_TxMethodCoverage(t *testing.T) {
	t.Parallel()

	// Every exported Tx method must be classified when detached transactions
	// are changed. This prevents a new method from silently falling through to
	// the detached transaction's fail-closed path.
	classified := make(map[string]struct{})
	for _, group := range [][]string{
		// Supported directly by the detached view.
		{"Event", "Range", "Dimension", "SetBlock", "Block", "BlockLoaded", "Liquid", "SetLiquid", "Redstone"},
		// Discarded presentation and scheduling side effects.
		{"ScheduleBlockUpdate", "AddParticle", "PlayEntityAnimation", "PlaySound"},
		// Unsupported operations that make the result unreliable.
		{
			"Defer", "DeferErr", "World", "SetBlockEntity", "BlocksWithin", "BuildStructure", "HighestLightBlocker", "HighestBlock", "Light", "SkyLight", "BlockLight",
			"SetBiome", "Biome", "Temperature", "RainingAt", "SnowingAt", "ThunderingAt", "Raining", "Thundering",
			"AddEntity", "AddEntityAt", "RemoveEntity", "EntitiesWithin", "Entities", "Players", "Viewers", "Sleepers",
			"BroadcastSleepingIndicator", "BroadcastSleepingReminder", "CurrentTick", "RedstonePower", "RedstoneDirectPower",
			"RedstoneStrongPower", "RedstoneConductivePower", "RedstonePowerFrom", "RedstoneDirectPowerFrom", "RedstoneStrongPowerFrom",
		},
	} {
		for _, name := range group {
			classified[name] = struct{}{}
		}
	}
	typ := reflect.TypeOf((*Tx)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if _, ok := classified[method.Name]; !ok {
			t.Errorf("Tx.%s is not classified for detached transactions", method.Name)
		}
	}
	for name := range classified {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("stale detached Tx classification for removed method %s", name)
		}
	}
}

type blockTransactionTestView struct {
	blocks map[cube.Pos]Block
}

func (*blockTransactionTestView) Range() cube.Range    { return Overworld.Range() }
func (*blockTransactionTestView) Dimension() Dimension { return Overworld }

func (v *blockTransactionTestView) Block(pos cube.Pos) Block {
	b, _ := v.BlockLoaded(pos)
	return b
}

func (v *blockTransactionTestView) BlockLoaded(pos cube.Pos) (Block, bool) {
	b, ok := v.blocks[pos]
	return b, ok
}

func (*blockTransactionTestView) Liquid(cube.Pos) (Liquid, bool) { return nil, false }

func (v *blockTransactionTestView) SetBlock(pos cube.Pos, b Block, _ *SetOpts) {
	v.blocks[pos] = b
}

func (*blockTransactionTestView) SetLiquid(cube.Pos, Liquid) {}

package item

// BreezeRod is an item dropped by breezes, which brews wind charged potions.
type BreezeRod struct{}

// EncodeItem ...
func (BreezeRod) EncodeItem() (name string, meta int16) {
	return "minecraft:breeze_rod", 0
}

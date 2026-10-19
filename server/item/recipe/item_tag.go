package recipe

import (
	"slices"

	vanilla "github.com/cinnabar-mc/protocolgen/generated/data/item"
)

var itemTags = vanillaItemTags()

// vanillaItemTags indexes the shared catalog's item memberships by recipe tag.
func vanillaItemTags() map[string][]string {
	tags := make(map[string][]string)
	for _, item := range vanilla.RuntimeItems() {
		for _, tag := range vanilla.RuntimeTags(item.Name) {
			tags[tag] = append(tags[tag], item.Name)
		}
	}
	return tags
}

// ItemTag represents a recipe item that is identified by a tag, such as "minecraft:planks" or
// "minecraft:digger" and so on.
type ItemTag struct {
	tag   string
	count int

	items []string
}

// NewItemTag creates a new item tag with the tag and count passed.
func NewItemTag(tag string, count int) ItemTag {
	if count < 0 {
		count = 0
	}
	return ItemTag{tag: tag, count: count, items: itemTags[tag]}
}

// ItemTags returns all vanilla item tags, indexed by tag name.
func ItemTags() map[string][]string {
	tags := make(map[string][]string, len(itemTags))
	for tag, items := range itemTags {
		tags[tag] = slices.Clone(items)
	}
	return tags
}

// ItemsInTag returns all item names in a vanilla item tag.
func ItemsInTag(tag string) []string {
	return slices.Clone(itemTags[tag])
}

// Count ...
func (i ItemTag) Count() int {
	return i.count
}

// Empty ...
func (i ItemTag) Empty() bool {
	return i.count == 0 || i.tag == ""
}

// Tag returns the tag of the item.
func (i ItemTag) Tag() string {
	return i.tag
}

// Contains returns true if the item tag contains the item with the name passed.
func (i ItemTag) Contains(name string) bool {
	return slices.Contains(i.items, name)
}

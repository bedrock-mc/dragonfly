package world

import "github.com/cinnabar-mc/protocolgen/generated/data/block"

// unknownBlockProperties holds the values used for blocks without an implementation.
type unknownBlockProperties struct {
	friction            float64
	dampening, emission uint8
}

var unknownBlockProps = vanillaBlockProperties()

// vanillaBlockProperties keeps the first state's friction and the lowest light
// values for each block name. Unknown blocks cannot model state-dependent light,
// so their shared fallback represents the block's resting value.
func vanillaBlockProperties() map[string]unknownBlockProperties {
	properties := make(map[string]unknownBlockProperties)
	for i := range block.StateCount() {
		state, _ := block.StateAt(i)
		definition, _ := block.BlockAt(state.Block)
		values, _ := block.PropertiesAt(state.Properties)
		previous, ok := properties[definition.Name]
		if !ok {
			properties[definition.Name] = unknownBlockProperties{
				friction: float64(values.Friction), dampening: values.LightDampening, emission: values.LightEmission,
			}
			continue
		}
		previous.dampening = min(previous.dampening, values.LightDampening)
		previous.emission = min(previous.emission, values.LightEmission)
		properties[definition.Name] = previous
	}
	return properties
}

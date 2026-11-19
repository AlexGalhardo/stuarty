package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

// processorParamsSchema is the schema of the params of audio.processor_add and audio.processor_set: numbers by name,
// with the table of the processors (every kind with its parameters, their unit, range and default) in the extension
// x-vv-processors, so that a form is drawn from the registry and states no range of its own (SDD-004 section 7.1).
func processorParamsSchema() map[string]any {
	kinds := make([]any, 0, len(timeline.ProcessorKinds))

	for _, kind := range timeline.ProcessorKinds {
		kinds = append(kinds, map[string]any{"kind": kind, "params": timeline.ProcessorParams(kind)})
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "number"},
		"x-vv-processors":      kinds,
		"x-vv-max-processors":  timeline.MaxProcessors,
	}
}

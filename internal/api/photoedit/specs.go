package photoedit

// ToolSpecs contains all supported tool specifications keyed by tool name.
var ToolSpecs = buildToolSpecs()

// buildToolSpecs merges tool specs from category-specific maps.
func buildToolSpecs() map[string]ToolSpec {
	result := make(map[string]ToolSpec)

	mergeToolSpecs(result, generationToolSpecs)
	mergeToolSpecs(result, technicalToolSpecs)
	mergeToolSpecs(result, workflowToolSpecs)

	return result
}

// mergeToolSpecs appends entries from source into target.
func mergeToolSpecs(target map[string]ToolSpec, source map[string]ToolSpec) {
	for name, spec := range source {
		target[name] = spec
	}
}

package photoedit

// ExecuteWithSpec routes runtime execution to the subagent specified in ToolSpec.
func ExecuteWithSpec(photoUID, tool string, params map[string]any, spec ToolSpec) (status, message string, err error) {
	switch spec.Subagent {
	case "generation-agent":
		return executeGenerationTool(photoUID, tool, params, spec)
	case "technical-agent":
		return executeTechnicalTool(photoUID, tool, params, spec)
	case "workflow-agent":
		return executeWorkflowTool(photoUID, tool, spec)
	default:
		return "failed", "unknown subagent", nil
	}
}

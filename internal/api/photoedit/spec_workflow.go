package photoedit

var workflowToolSpecs = map[string]ToolSpec{
	"version_checkpoint": {Category: "workflow", Subagent: "workflow-agent", Executable: true},
	"metadata_sync":      {Category: "workflow", Subagent: "workflow-agent", Executable: true},
}

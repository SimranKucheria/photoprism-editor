package photoedit

var generationToolSpecs = map[string]ToolSpec{
	"inpaint_object":     {Category: "manipulation", Subagent: "generation-agent", Executable: true},
	"outpaint_canvas":    {Category: "manipulation", Subagent: "generation-agent", Executable: true},
	"transfer_style":     {Category: "manipulation", Subagent: "generation-agent", Executable: true},
	"replace_background": {Category: "manipulation", Subagent: "generation-agent", Executable: true},
}

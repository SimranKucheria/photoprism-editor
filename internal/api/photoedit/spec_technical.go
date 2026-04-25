package photoedit

var technicalToolSpecs = map[string]ToolSpec{
	"auto_crop_straighten": {Category: "technical", Subagent: "technical-agent", Executable: true},
	"crop":                 {Category: "technical", Subagent: "technical-agent", Executable: true},
	"rotate":               {Category: "technical", Subagent: "technical-agent", Executable: true},
	"exposure":             {Category: "technical", Subagent: "technical-agent", Executable: true},
	"contrast":             {Category: "technical", Subagent: "technical-agent", Executable: true},
	"saturation":           {Category: "technical", Subagent: "technical-agent", Executable: true},
	"temperature":          {Category: "technical", Subagent: "technical-agent", Executable: true},
}

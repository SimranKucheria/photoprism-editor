package photoedit

import (
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/photoprism/photoprism/pkg/clean"
)

// Operation represents a single validated operation in an edit plan.
type Operation struct {
	Tool     string         `json:"tool"`
	Subagent string         `json:"subagent,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

// ParseAndValidateToolOps parses planner JSON and validates operations.
func ParseAndValidateToolOps(raw string) ([]Operation, error) {
	raw = clean.JSON(strings.TrimSpace(raw))
	if raw == "" {
		return nil, errors.New("empty planner response")
	}

	var payload struct {
		Operations []Operation `json:"operations"`
	}

	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	validated := make([]Operation, 0, len(payload.Operations))

	for _, op := range payload.Operations {
		if valid := validateToolOp(op); valid != nil {
			validated = append(validated, *valid)
		}
	}

	if len(validated) == 0 {
		return nil, errors.New("no valid operations")
	}

	if len(validated) > 6 {
		validated = validated[:6]
	}

	return validated, nil
}

// validateToolOp sanitizes a planned operation to the allowed MVP schema.
func validateToolOp(op Operation) *Operation {
	tool := strings.ToLower(strings.TrimSpace(op.Tool))
	if tool == "" {
		return nil
	}

	spec, ok := ToolSpecs[tool]
	if !ok {
		return nil
	}

	params := map[string]any{}
	for k, v := range op.Params {
		params[strings.TrimSpace(k)] = v
	}

	if tool == "rotate" {
		if degrees, ok := floatParam(params, "degrees"); ok {
			degrees = math.Mod(degrees, 360)
			if degrees > 180 {
				degrees -= 360
			} else if degrees < -180 {
				degrees += 360
			}
			params["degrees"] = degrees
		}
	}

	if tool == "ai_enhance" {
		if strength, ok := params["strength"].(string); ok {
			strength = strings.ToLower(strings.TrimSpace(strength))
			switch strength {
			case "low", "normal", "high":
				params["strength"] = strength
			default:
				params["strength"] = "normal"
			}
		} else {
			params["strength"] = "normal"
		}
	}

	subagent := strings.ToLower(strings.TrimSpace(op.Subagent))
	if subagent == "" || subagent != spec.Subagent {
		subagent = spec.Subagent
	}

	return &Operation{Tool: tool, Subagent: subagent, Params: params}
}

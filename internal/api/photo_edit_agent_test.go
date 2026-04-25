package api

import (
	"testing"

	"github.com/photoprism/photoprism/internal/api/photoedit"
	"github.com/stretchr/testify/require"
)

// TestCleanPrompt verifies user prompt sanitization.
func TestCleanPrompt(t *testing.T) {
	actual := cleanPrompt("  Increase exposure and crop   ")
	require.Equal(t, "Increase exposure and crop", actual)
}

// TestParseAndValidateToolOps verifies planner JSON parsing and schema validation.
func TestParseAndValidateToolOps(t *testing.T) {
	t.Run("ValidOperations", func(t *testing.T) {
		ops, err := photoedit.ParseAndValidateToolOps(`{"operations":[{"tool":"rotate","params":{"degrees":270}},{"tool":"exposure","params":{"value":0.4}}]}`)
		require.NoError(t, err)
		require.Len(t, ops, 2)
		require.Equal(t, "rotate", ops[0].Tool)
		require.Equal(t, "technical-agent", ops[0].Subagent)
		require.Equal(t, -90.0, ops[0].Params["degrees"])
		require.Equal(t, "exposure", ops[1].Tool)
		require.Equal(t, "technical-agent", ops[1].Subagent)
	})

	t.Run("RejectUnknownTools", func(t *testing.T) {
		_, err := photoedit.ParseAndValidateToolOps(`{"operations":[{"tool":"delete_originals","params":{}}]}`)
		require.Error(t, err)
	})
}

// TestPlannerModel verifies planner model override behavior.
func TestPlannerModel(t *testing.T) {
	t.Run("UsesEnvOverride", func(t *testing.T) {
		t.Setenv(editAIOllamaModelEnv, "qwen2.5vl:7b")
		require.Equal(t, "qwen2.5vl:7b", plannerModel())
	})

	t.Run("UsesDefaultWhenUnset", func(t *testing.T) {
		t.Setenv(editAIOllamaModelEnv, "")
		require.Equal(t, defaultEditAIOllamaModel, plannerModel())
	})
}

// TestOllamaErrorMessage verifies extraction and formatting of upstream errors.
func TestOllamaErrorMessage(t *testing.T) {
	t.Run("ParsesJSONError", func(t *testing.T) {
		msg := ollamaErrorMessage(404, []byte(`{"error":"model 'gemma3:latest' not found"}`))
		require.Contains(t, msg, "Not Found")
		require.Contains(t, msg, "not found")
	})

	t.Run("FallsBackToBody", func(t *testing.T) {
		msg := ollamaErrorMessage(502, []byte("bad gateway"))
		require.Contains(t, msg, "Bad Gateway")
		require.Contains(t, msg, "bad gateway")
	})
}

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/photoprism/photoprism/internal/ai/vision/ollama"
	"github.com/photoprism/photoprism/internal/api/photoedit"
	"github.com/photoprism/photoprism/internal/auth/acl"
	"github.com/photoprism/photoprism/internal/entity/query"
	"github.com/photoprism/photoprism/internal/event"
	"github.com/photoprism/photoprism/internal/photoprism/get"
	"github.com/photoprism/photoprism/pkg/clean"
	"github.com/photoprism/photoprism/pkg/http/safe"
	"github.com/photoprism/photoprism/pkg/i18n"
	"github.com/photoprism/photoprism/pkg/rnd"
)

var photoEditPlanStore = &editPlanStore{plans: make(map[string]*photoEditPlan)}

var photoEditAIPlanner = newPhotoEditAIPlanner()

const editAIOllamaModelEnv = "PHOTOPRISM_EDIT_AI_MODEL"

const defaultEditAIOllamaModel = "llama3.2:3b"

// editPlanStore stores short-lived edit plans for agent execution and status lookup.
type editPlanStore struct {
	mu    sync.RWMutex
	plans map[string]*photoEditPlan
}

// photoEditPlan stores in-memory plan state for initial agent workflow scaffolding.
type photoEditPlan struct {
	ID         string                `json:"id"`
	PhotoUID   string                `json:"photoUid"`
	Prompt     string                `json:"prompt"`
	Operations []photoEditToolOp     `json:"operations"`
	Results    []photoEditToolResult `json:"results,omitempty"`
	Approved   bool                  `json:"approved"`
	Status     string                `json:"status"`
	CreatedAt  time.Time             `json:"createdAt"`
	UpdatedAt  time.Time             `json:"updatedAt"`
}

// photoEditToolOp represents a single validated operation in an edit plan.
type photoEditToolOp struct {
	Tool     string         `json:"tool"`
	Subagent string         `json:"subagent,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

// photoEditToolResult stores execution outcomes for each operation.
type photoEditToolResult struct {
	Tool     string `json:"tool"`
	Subagent string `json:"subagent,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// photoEditPlanRequest contains prompt input to generate an edit plan.
type photoEditPlanRequest struct {
	Prompt string `json:"prompt"`
}

// photoEditPlanIDRequest contains a plan id used by plan lifecycle endpoints.
type photoEditPlanIDRequest struct {
	PlanID string `json:"planId"`
}

// photoEditPlanResponse contains plan metadata returned to the frontend.
type photoEditPlanResponse struct {
	PlanID     string                `json:"planId"`
	PhotoUID   string                `json:"photoUid"`
	Prompt     string                `json:"prompt"`
	Operations []photoEditToolOp     `json:"operations"`
	Results    []photoEditToolResult `json:"results,omitempty"`
	Approved   bool                  `json:"approved"`
	Status     string                `json:"status"`
	CreatedAt  time.Time             `json:"createdAt"`
	UpdatedAt  time.Time             `json:"updatedAt"`
}

// PhotoEditPlan parses an NLP prompt and creates a bounded non-destructive edit plan.
func PhotoEditPlan(router *gin.RouterGroup) {
	router.POST("/photos/:uid/edit/plan", func(c *gin.Context) {
		s := Auth(c, acl.ResourcePhotos, acl.ActionUpdate)

		if s.Abort(c) {
			return
		}

		conf := get.Config()

		if conf.ReadOnly() || !conf.Settings().Features.Edit {
			c.AbortWithStatusJSON(http.StatusForbidden, i18n.NewResponse(http.StatusForbidden, i18n.ErrReadOnly))
			return
		}

		uid := clean.UID(c.Param("uid"))

		if _, err := query.PhotoByUID(uid); err != nil {
			AbortEntityNotFound(c)
			return
		}

		var frm photoEditPlanRequest

		LimitRequestBodyBytes(c, MaxMutationRequestBytes)

		if err := c.BindJSON(&frm); err != nil {
			if IsRequestBodyTooLarge(err) {
				AbortRequestTooLarge(c, i18n.ErrBadRequest)
				return
			}

			AbortBadRequest(c, err)
			return
		}

		prompt := cleanPrompt(frm.Prompt)

		if prompt == "" {
			Abort(c, http.StatusBadRequest, i18n.ErrBadRequest)
			return
		}

		operations, plannerErr := photoEditAIPlanner.Plan(prompt)
		if plannerErr != nil {
			log.Debugf("api: ai planner failed for %s (%s)", clean.Log(uid), clean.Error(plannerErr))
			Abort(c, http.StatusServiceUnavailable, i18n.ErrConnectionFailed)
			return
		}

		plan := &photoEditPlan{
			ID:         rnd.UUIDv7(),
			PhotoUID:   uid,
			Prompt:     prompt,
			Operations: operations,
			Approved:   true,
			Status:     "running",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}

		results := executePhotoEditPlanTools(plan.PhotoUID, plan.Operations)
		plan.Results = results

		succeeded := 0
		for _, result := range results {
			if result.Status == "succeeded" {
				succeeded++
			}
		}

		switch {
		case succeeded == len(results) && len(results) > 0:
			plan.Status = "applied"
			event.SuccessMsg(i18n.MsgChangesSaved)
		case succeeded > 0:
			plan.Status = "partially-applied"
		case len(results) == 0:
			plan.Status = "failed"
		default:
			plan.Status = "failed"
		}

		plan.UpdatedAt = time.Now().UTC()

		photoEditPlanStore.set(plan)

		if succeeded > 0 {
			PublishPhotoEvent(StatusUpdated, plan.PhotoUID, c)
		}

		c.JSON(http.StatusOK, toPhotoEditPlanResponse(plan))
	})
}

// PhotoEditStatus returns the current plan status.
func PhotoEditStatus(router *gin.RouterGroup) {
	router.GET("/photos/:uid/edit/status/:plan_id", func(c *gin.Context) {
		s := Auth(c, acl.ResourcePhotos, acl.ActionView)

		if s.Abort(c) {
			return
		}

		uid := clean.UID(c.Param("uid"))
		planID := strings.ToLower(strings.TrimSpace(c.Param("plan_id")))

		plan, ok := photoEditPlanStore.get(planID)

		if !ok || plan.PhotoUID != uid {
			AbortEntityNotFound(c)
			return
		}

		c.JSON(http.StatusOK, toPhotoEditPlanResponse(plan))
	})
}

// PhotoEditReset deletes a plan so users can start over.
func PhotoEditReset(router *gin.RouterGroup) {
	router.POST("/photos/:uid/edit/reset", func(c *gin.Context) {
		s := Auth(c, acl.ResourcePhotos, acl.ActionUpdate)

		if s.Abort(c) {
			return
		}

		uid := clean.UID(c.Param("uid"))

		var frm photoEditPlanIDRequest

		LimitRequestBodyBytes(c, MaxMutationRequestBytes)

		if err := c.BindJSON(&frm); err != nil {
			if IsRequestBodyTooLarge(err) {
				AbortRequestTooLarge(c, i18n.ErrBadRequest)
				return
			}

			AbortBadRequest(c, err)
			return
		}

		planID := strings.ToLower(strings.TrimSpace(frm.PlanID))
		plan, ok := photoEditPlanStore.get(planID)

		if !ok || plan.PhotoUID != uid {
			AbortEntityNotFound(c)
			return
		}

		photoEditPlanStore.delete(planID)

		c.JSON(http.StatusOK, gin.H{"planId": planID, "status": "reset"})
	})
}

// cleanPrompt sanitizes prompt text before plan generation.
func cleanPrompt(prompt string) string {
	return strings.TrimSpace(clean.TypeUnicode(prompt))
}

// photoEditPlanner describes planning behavior for prompt-to-tools conversion.
type photoEditPlanner interface {
	Plan(prompt string) ([]photoEditToolOp, error)
}

// aiPhotoEditPlanner generates plans through the Ollama provider.
type aiPhotoEditPlanner struct {
	ollama *ollamaPlannerClient
}

// ollamaPlannerClient stores configuration for Ollama prompt planning.
type ollamaPlannerClient struct {
	url   string
	model string
	http  *http.Client
}

// newPhotoEditAIPlanner creates the planner strategy using Ollama.
func newPhotoEditAIPlanner() photoEditPlanner {
	planner := &aiPhotoEditPlanner{
		ollama: &ollamaPlannerClient{
			url:   strings.TrimRight(strings.TrimSpace(expandOrDefault(ollama.BaseUrlEnv, ollama.DefaultBaseUrl)), "/") + "/api/generate",
			model: plannerModel(),
			http:  &http.Client{Timeout: 600 * time.Second},
		},
	}

	return planner
}

// Plan resolves prompt text into validated tool operations using Ollama.
func (p *aiPhotoEditPlanner) Plan(prompt string) ([]photoEditToolOp, error) {
	if p == nil {
		return nil, errors.New("planner is nil")
	}

	return p.planOllama(prompt)
}

// planOllama requests a JSON tool plan from an Ollama-compatible model.
func (p *aiPhotoEditPlanner) planOllama(prompt string) ([]photoEditToolOp, error) {
	if p.ollama == nil {
		return nil, errors.New("ollama planner not configured")
	}

	if _, err := safe.URL(p.ollama.url); err != nil {
		return nil, errors.New("invalid ollama url")
	}

	body := map[string]any{
		"model":  p.ollama.model,
		"stream": false,
		"prompt": plannerPrompt(prompt),
		"format": "json",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, p.ollama.url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.ollama.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, errors.New("ollama planning request failed: " + ollamaErrorMessage(resp.StatusCode, raw))
	}

	var payload struct {
		Response string `json:"response"`
	}

	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	validated, err := photoedit.ParseAndValidateToolOps(payload.Response)
	if err != nil {
		return nil, err
	}

	result := make([]photoEditToolOp, 0, len(validated))
	for _, op := range validated {
		result = append(result, photoEditToolOp{Tool: op.Tool, Subagent: op.Subagent, Params: op.Params})
	}

	return result, nil
}

// executePhotoEditPlanTools executes supported tools and returns per-tool outcomes.
func executePhotoEditPlanTools(photoUID string, ops []photoEditToolOp) []photoEditToolResult {
	results := make([]photoEditToolResult, 0, len(ops))

	for _, op := range ops {
		tool := strings.ToLower(strings.TrimSpace(op.Tool))
		if tool == "" {
			continue
		}

		result := executeToolWithSubagent(photoUID, op)

		results = append(results, result)
	}

	return results
}

// executeToolWithSubagent routes execution to the owning subagent.
func executeToolWithSubagent(photoUID string, op photoEditToolOp) photoEditToolResult {
	tool := strings.ToLower(strings.TrimSpace(op.Tool))
	result := photoEditToolResult{Tool: tool, Subagent: op.Subagent, Status: "failed"}

	spec, ok := photoedit.ToolSpecs[tool]
	if !ok {
		result.Message = "unknown tool"
		return result
	}

	if result.Subagent == "" {
		result.Subagent = spec.Subagent
	}

	status, message, err := photoedit.ExecuteWithSpec(photoUID, tool, op.Params, spec)
	if err != nil {
		result.Message = clean.Error(err)
		return result
	}

	result.Status = status
	result.Message = message

	return result
}

// expandOrDefault returns the expanded environment value or fallback default.
func expandOrDefault(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return os.ExpandEnv(v)
}

// plannerModel resolves planner model from env with a lightweight default.
func plannerModel() string {
	if v := strings.TrimSpace(os.Getenv(editAIOllamaModelEnv)); v != "" {
		return os.ExpandEnv(v)
	}

	return defaultEditAIOllamaModel
}

// ollamaErrorMessage returns a concise upstream error message from Ollama responses.
func ollamaErrorMessage(statusCode int, raw []byte) string {
	message := strings.TrimSpace(string(raw))

	var payload struct {
		Error string `json:"error"`
	}

	if err := json.Unmarshal(raw, &payload); err == nil {
		if payload.Error != "" {
			message = strings.TrimSpace(payload.Error)
		}
	}

	if message == "" {
		return "status " + http.StatusText(statusCode)
	}

	if len(message) > 220 {
		message = strings.TrimSpace(message[:220]) + "..."
	}

	return "status " + http.StatusText(statusCode) + ": " + message
}

// plannerSystemPrompt defines the strict planning role for model-based tool generation.
func plannerSystemPrompt() string {
	return "You are a PhotoPrism NLP-to-tool planner for an agentic photo editor. Return strict JSON with key operations (array). Each operation MUST include tool, subagent, and params. Subagents: generation-agent, technical-agent, workflow-agent. Allowed tools: inpaint_object, outpaint_canvas, transfer_style, replace_background, auto_crop_straighten, crop, rotate, exposure, contrast, saturation, temperature, version_checkpoint, metadata_sync. Never return markdown." //nolint:lll
}

// plannerPrompt wraps user text with schema requirements for Ollama generation.
func plannerPrompt(userPrompt string) string {
	return plannerSystemPrompt() + " User request: " + userPrompt
}

// toPhotoEditPlanResponse converts internal plan state into response payload shape.
func toPhotoEditPlanResponse(plan *photoEditPlan) photoEditPlanResponse {
	return photoEditPlanResponse{
		PlanID:     plan.ID,
		PhotoUID:   plan.PhotoUID,
		Prompt:     plan.Prompt,
		Operations: plan.Operations,
		Results:    plan.Results,
		Approved:   plan.Approved,
		Status:     plan.Status,
		CreatedAt:  plan.CreatedAt,
		UpdatedAt:  plan.UpdatedAt,
	}
}

// get returns plan state by id.
func (s *editPlanStore) get(id string) (*photoEditPlan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	plan, ok := s.plans[id]

	return plan, ok
}

// set upserts plan state by id.
func (s *editPlanStore) set(plan *photoEditPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.plans[plan.ID] = plan
}

// delete removes plan state by id.
func (s *editPlanStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.plans, id)
}

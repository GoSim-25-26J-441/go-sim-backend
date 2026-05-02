package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/hostconfig"
	asrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/analysis_suggestions/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/amg_apd_scenario"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/gin-gonic/gin"
)

func serverScenarioError(c *gin.Context, msg string, err error) {
	log.Printf("simulation scenario: %s: %v", msg, err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg, "details": err.Error()})
}

func cloneMetadata(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (h *Handler) putScenarioToS3(ctx context.Context, diagramVersionID, scenarioYAML string) string {
	if h.s3Client == nil || diagramVersionID == "" {
		return ""
	}
	key := "simulation/diagram_versions/" + diagramVersionID + "/scenario.yaml"
	if err := h.s3Client.PutObject(ctx, key, []byte(scenarioYAML)); err != nil {
		return ""
	}
	return key
}

// useCachedGeneratedScenario returns true when cached row should be used without regenerating from AMG/APD.
func useCachedGeneratedScenario(cached *simrepo.CachedScenario, generationSourceHash string) bool {
	if cached == nil || strings.TrimSpace(cached.ScenarioYAML) == "" {
		return false
	}
	if cached.Source == "edited" {
		return true
	}
	if generationSourceHash != "" && cached.SourceHash != "" && cached.SourceHash != generationSourceHash {
		return false
	}
	return true
}

// scenarioGenerationOptsAndHash loads optional host sizing from analysis-suggestions request_responses,
// builds placement hosts for scenario generation, and returns hashes for diagram-only vs combined generation source.
// A non-nil error indicates a database failure while loading request_responses (not “no config”); callers must fail the request.
func (h *Handler) scenarioGenerationOptsAndHash(ctx context.Context, userID, projectID, amgYAML string) (
	opts amg_apd_scenario.GenerationOptions,
	diagramHash string,
	generationHash string,
	err error,
) {
	diagramHash = simrepo.HashAMGAPDSource(amgYAML)
	generationHash = diagramHash
	if h.db == nil || strings.TrimSpace(projectID) == "" {
		return amg_apd_scenario.GenerationOptions{}, diagramHash, generationHash, nil
	}
	cfg, ok, loadErr := asrepo.LoadLatestScenarioHostConfig(ctx, h.db, userID, projectID)
	if loadErr != nil {
		return amg_apd_scenario.GenerationOptions{}, diagramHash, diagramHash, loadErr
	}
	if !ok {
		return amg_apd_scenario.GenerationOptions{}, diagramHash, generationHash, nil
	}
	hosts := amg_apd_scenario.HostDocsFromCounts(cfg.Nodes, cfg.Cores, cfg.MemoryGB)
	if len(hosts) == 0 {
		return amg_apd_scenario.GenerationOptions{}, diagramHash, generationHash, nil
	}
	opts = amg_apd_scenario.GenerationOptions{Hosts: hosts}
	generationHash = simrepo.HashScenarioGenerationSource(amgYAML, hostconfig.CanonicalJSON(cfg))
	return opts, diagramHash, generationHash, nil
}

// resolveScenarioYAMLForDiagramVersion loads or generates scenario YAML for a diagram version.
// It may persist a generated scenario. Edited scenarios are always returned as-is for runs.
func (h *Handler) resolveScenarioYAMLForDiagramVersion(ctx context.Context, userID, projectID, diagramVersionID string) (effectiveYAML string, diagramSourceHash string, cached *simrepo.CachedScenario, err error) {
	if h.scenarioCacheRepo == nil {
		return "", "", nil, errors.New("scenario cache repository not available")
	}
	amgYAML, err := h.scenarioCacheRepo.GetDiagramYAMLContent(ctx, userID, projectID, diagramVersionID)
	if err != nil {
		return "", "", nil, err
	}
	diagramHash := simrepo.HashAMGAPDSource(amgYAML)
	cached, err = h.scenarioCacheRepo.GetScenarioForDiagramVersion(ctx, userID, projectID, diagramVersionID)
	if err != nil {
		return "", diagramHash, nil, err
	}
	if cached != nil && cached.Source == "edited" {
		return cached.ScenarioYAML, diagramHash, cached, nil
	}
	opts, _, generationHash, genOptErr := h.scenarioGenerationOptsAndHash(ctx, userID, projectID, amgYAML)
	if genOptErr != nil {
		return "", diagramHash, nil, genOptErr
	}
	if useCachedGeneratedScenario(cached, generationHash) {
		return cached.ScenarioYAML, generationHash, cached, nil
	}
	genYAML, err := amg_apd_scenario.GenerateScenarioYAMLWithOptions([]byte(amgYAML), opts)
	if err != nil {
		return "", diagramHash, cached, err
	}
	if _, err := h.validateScenarioDraft(ctx, genYAML); err != nil {
		return "", diagramHash, cached, err
	}
	sh := generationHash
	s3Path := h.putScenarioToS3(ctx, diagramVersionID, genYAML)
	overwrite := cached != nil
	out, err := h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(ctx, diagramVersionID, genYAML, "generated", s3Path, &sh, overwrite)
	if err != nil {
		return "", diagramHash, cached, err
	}
	return genYAML, generationHash, out, nil
}

// GetDiagramVersionScenario returns cached or freshly generated scenario YAML for editing/runs.
func (h *Handler) GetDiagramVersionScenario(c *gin.Context) {
	projectID := c.Param("project_id")
	diagramVersionID := c.Param("diagram_version_id")
	if projectID == "" || diagramVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and diagram_version_id are required"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}
	if h.scenarioCacheRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scenario cache not configured"})
		return
	}
	if err := h.scenarioCacheRepo.VerifyDiagramVersionForProject(c.Request.Context(), userID, projectID, diagramVersionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
		return
	}
	amgYAML, err := h.scenarioCacheRepo.GetDiagramYAMLContent(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		if errors.Is(err, simrepo.ErrDiagramMissingYAML) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "diagram version has no stored AMG/APD YAML"})
			return
		}
		if errors.Is(err, simrepo.ErrDiagramVersionNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
			return
		}
		serverScenarioError(c, "failed to load diagram YAML", err)
		return
	}
	diagramHash := simrepo.HashAMGAPDSource(amgYAML)
	cached, err := h.scenarioCacheRepo.GetScenarioForDiagramVersion(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		serverScenarioError(c, "failed to load cached scenario", err)
		return
	}
	if cached != nil && cached.Source == "edited" && cached.SourceHash != "" && cached.SourceHash != diagramHash {
		c.JSON(http.StatusConflict, gin.H{
			"error": "diagram AMG/APD content changed since the scenario was last saved; use PUT with the updated scenario or POST .../regenerate with overwrite=true",
		})
		return
	}
	if cached != nil && cached.Source == "edited" {
		sum := sha256.Sum256([]byte(cached.ScenarioYAML))
		c.JSON(http.StatusOK, gin.H{
			"scenario_yaml":       cached.ScenarioYAML,
			"scenario_hash":       hex.EncodeToString(sum[:]),
			"source":              cached.Source,
			"source_hash":         cached.SourceHash,
			"diagram_source_hash": diagramHash,
			"s3_path":             cached.S3Path,
			"updated_at":          cached.UpdatedAt,
		})
		return
	}
	opts, _, generationHash, genOptErr := h.scenarioGenerationOptsAndHash(c.Request.Context(), userID, projectID, amgYAML)
	if genOptErr != nil {
		serverScenarioError(c, "failed to load analysis host configuration", genOptErr)
		return
	}
	if useCachedGeneratedScenario(cached, generationHash) && cached != nil {
		sum := sha256.Sum256([]byte(cached.ScenarioYAML))
		c.JSON(http.StatusOK, gin.H{
			"scenario_yaml":       cached.ScenarioYAML,
			"scenario_hash":       hex.EncodeToString(sum[:]),
			"source":              cached.Source,
			"source_hash":         cached.SourceHash,
			"diagram_source_hash": generationHash,
			"s3_path":             cached.S3Path,
			"updated_at":          cached.UpdatedAt,
		})
		return
	}
	genYAML, genErr := amg_apd_scenario.GenerateScenarioYAMLWithOptions([]byte(amgYAML), opts)
	if genErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to generate valid scenario from AMG/APD YAML", "details": genErr.Error()})
		return
	}
	valRes, err := h.validateScenarioDraft(c.Request.Context(), genYAML)
	if err != nil {
		h.writeScenarioValidationError(c, err, genYAML)
		return
	}
	sh := generationHash
	s3Path := h.putScenarioToS3(c.Request.Context(), diagramVersionID, genYAML)
	overwrite := cached != nil
	saved, upErr := h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(c.Request.Context(), diagramVersionID, genYAML, "generated", s3Path, &sh, overwrite)
	if upErr != nil {
		if errors.Is(upErr, simrepo.ErrScenarioCacheConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "scenario cache conflict; set overwrite=true on regenerate"})
			return
		}
		serverScenarioError(c, "failed to persist scenario", upErr)
		return
	}
	sum := sha256.Sum256([]byte(genYAML))
	c.JSON(http.StatusOK, gin.H{
		"scenario_yaml":       genYAML,
		"scenario_hash":       hex.EncodeToString(sum[:]),
		"source":              saved.Source,
		"source_hash":         saved.SourceHash,
		"diagram_source_hash": generationHash,
		"s3_path":             saved.S3Path,
		"updated_at":          saved.UpdatedAt,
		"validation":          valRes,
	})
}

// PutDiagramVersionScenario validates and persists an edited scenario for a diagram version.
func (h *Handler) PutDiagramVersionScenario(c *gin.Context) {
	projectID := c.Param("project_id")
	diagramVersionID := c.Param("diagram_version_id")
	if projectID == "" || diagramVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and diagram_version_id are required"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}
	var body struct {
		ScenarioYAML string `json:"scenario_yaml"`
		Overwrite    bool   `json:"overwrite"`
		Mode         string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(body.ScenarioYAML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario_yaml is required"})
		return
	}
	validationMode, modeErr := ParseScenarioValidationEditorMode(body.Mode, ScenarioValidateModeDraft)
	if modeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": modeErr.Error()})
		return
	}
	var valRes *ScenarioValidationResult
	var err error
	if validationMode == ScenarioValidateModePreflight {
		valRes, err = h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML)
	} else {
		valRes, err = h.validateScenarioDraft(c.Request.Context(), body.ScenarioYAML)
	}
	if err != nil {
		h.writeScenarioValidationError(c, err)
		return
	}
	if h.scenarioCacheRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scenario cache not configured"})
		return
	}
	if err := h.scenarioCacheRepo.VerifyDiagramVersionForProject(c.Request.Context(), userID, projectID, diagramVersionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
		return
	}
	amgYAML, err := h.scenarioCacheRepo.GetDiagramYAMLContent(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil && !errors.Is(err, simrepo.ErrDiagramMissingYAML) {
		if errors.Is(err, simrepo.ErrDiagramVersionNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
			return
		}
		serverScenarioError(c, "failed to load diagram YAML", err)
		return
	}
	var shPtr *string
	if err == nil {
		sh := simrepo.HashAMGAPDSource(amgYAML)
		shPtr = &sh
	}
	s3Path := h.putScenarioToS3(c.Request.Context(), diagramVersionID, body.ScenarioYAML)
	saved, err := h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(c.Request.Context(), diagramVersionID, body.ScenarioYAML, "edited", s3Path, shPtr, body.Overwrite)
	if err != nil {
		if errors.Is(err, simrepo.ErrScenarioCacheConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "scenario cache conflict; set overwrite=true to replace"})
			return
		}
		serverScenarioError(c, "failed to persist scenario", err)
		return
	}
	sum := sha256.Sum256([]byte(saved.ScenarioYAML))
	c.JSON(http.StatusOK, gin.H{
		"scenario_yaml": saved.ScenarioYAML,
		"scenario_hash": hex.EncodeToString(sum[:]),
		"source":        saved.Source,
		"source_hash":   saved.SourceHash,
		"s3_path":       saved.S3Path,
		"updated_at":    saved.UpdatedAt,
		"validation":    valRes,
	})
}

// PostValidateDiagramVersionScenario validates provided scenario YAML for a diagram version without persisting any changes.
func (h *Handler) PostValidateDiagramVersionScenario(c *gin.Context) {
	projectID := c.Param("project_id")
	diagramVersionID := c.Param("diagram_version_id")
	if projectID == "" || diagramVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and diagram_version_id are required"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}
	var body struct {
		ScenarioYAML string `json:"scenario_yaml"`
		Mode         string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(body.ScenarioYAML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario_yaml is required"})
		return
	}
	validationMode, modeErr := ParseScenarioValidationEditorMode(body.Mode, ScenarioValidateModeDraft)
	if modeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": modeErr.Error()})
		return
	}
	if h.scenarioCacheRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scenario cache not configured"})
		return
	}
	if err := h.scenarioCacheRepo.VerifyDiagramVersionForProject(c.Request.Context(), userID, projectID, diagramVersionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
		return
	}
	var valRes *ScenarioValidationResult
	var err error
	if validationMode == ScenarioValidateModePreflight {
		valRes, err = h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML)
	} else {
		valRes, err = h.validateScenarioDraft(c.Request.Context(), body.ScenarioYAML)
	}
	if err != nil {
		if h.writeScenarioValidationError(c, err) {
			return
		}
		serverScenarioError(c, "scenario validation failed", err)
		return
	}
	c.JSON(http.StatusOK, valRes)
}

// PostRegenerateDiagramVersionScenario regenerates scenario YAML from stored AMG/APD YAML.
func (h *Handler) PostRegenerateDiagramVersionScenario(c *gin.Context) {
	projectID := c.Param("project_id")
	diagramVersionID := c.Param("diagram_version_id")
	if projectID == "" || diagramVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and diagram_version_id are required"})
		return
	}
	userID := c.GetString("firebase_uid")
	if userID == "" {
		userID = c.GetHeader("X-User-Id")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
	}
	var body struct {
		Overwrite bool `json:"overwrite"`
	}
	_ = c.ShouldBindJSON(&body)

	if h.scenarioCacheRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scenario cache not configured"})
		return
	}
	if err := h.scenarioCacheRepo.VerifyDiagramVersionForProject(c.Request.Context(), userID, projectID, diagramVersionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
		return
	}
	amgYAML, err := h.scenarioCacheRepo.GetDiagramYAMLContent(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		if errors.Is(err, simrepo.ErrDiagramMissingYAML) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "diagram version has no stored AMG/APD YAML"})
			return
		}
		if errors.Is(err, simrepo.ErrDiagramVersionNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagram_version_id for project/user"})
			return
		}
		serverScenarioError(c, "failed to load diagram YAML", err)
		return
	}
	cached, err := h.scenarioCacheRepo.GetScenarioForDiagramVersion(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		serverScenarioError(c, "failed to load cached scenario", err)
		return
	}
	if cached != nil && cached.Source == "edited" && !body.Overwrite {
		c.JSON(http.StatusConflict, gin.H{"error": "an edited scenario exists; pass overwrite=true to replace it with a generated draft"})
		return
	}
	opts, _, generationHash, genOptErr := h.scenarioGenerationOptsAndHash(c.Request.Context(), userID, projectID, amgYAML)
	if genOptErr != nil {
		serverScenarioError(c, "failed to load analysis host configuration", genOptErr)
		return
	}
	genYAML, genErr := amg_apd_scenario.GenerateScenarioYAMLWithOptions([]byte(amgYAML), opts)
	if genErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to generate valid scenario from AMG/APD YAML", "details": genErr.Error()})
		return
	}
	valRes, err := h.validateScenarioDraft(c.Request.Context(), genYAML)
	if err != nil {
		h.writeScenarioValidationError(c, err, genYAML)
		return
	}
	sh := generationHash
	s3Path := h.putScenarioToS3(c.Request.Context(), diagramVersionID, genYAML)
	saved, upErr := h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(c.Request.Context(), diagramVersionID, genYAML, "generated", s3Path, &sh, true)
	if upErr != nil {
		serverScenarioError(c, "failed to persist scenario", upErr)
		return
	}
	sum := sha256.Sum256([]byte(genYAML))
	c.JSON(http.StatusOK, gin.H{
		"scenario_yaml":       genYAML,
		"scenario_hash":       hex.EncodeToString(sum[:]),
		"source":              saved.Source,
		"source_hash":         saved.SourceHash,
		"diagram_source_hash": generationHash,
		"s3_path":             saved.S3Path,
		"updated_at":          saved.UpdatedAt,
		"validation":          valRes,
	})
}

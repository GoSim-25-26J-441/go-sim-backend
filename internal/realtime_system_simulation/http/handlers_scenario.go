package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

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
func useCachedGeneratedScenario(cached *simrepo.CachedScenario, diagramSourceHash string) bool {
	if cached == nil || strings.TrimSpace(cached.ScenarioYAML) == "" {
		return false
	}
	if cached.Source == "edited" {
		return true
	}
	if diagramSourceHash != "" && cached.SourceHash != "" && cached.SourceHash != diagramSourceHash {
		return false
	}
	return true
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
	diagramSourceHash = simrepo.HashAMGAPDSource(amgYAML)
	cached, err = h.scenarioCacheRepo.GetScenarioForDiagramVersion(ctx, userID, projectID, diagramVersionID)
	if err != nil {
		return "", diagramSourceHash, nil, err
	}
	if cached != nil && cached.Source == "edited" {
		return cached.ScenarioYAML, diagramSourceHash, cached, nil
	}
	if useCachedGeneratedScenario(cached, diagramSourceHash) {
		return cached.ScenarioYAML, diagramSourceHash, cached, nil
	}
	genYAML, err := amg_apd_scenario.GenerateScenarioYAML([]byte(amgYAML))
	if err != nil {
		return "", diagramSourceHash, cached, err
	}
	if _, err := h.validateScenarioPreflight(ctx, genYAML); err != nil {
		return "", diagramSourceHash, cached, err
	}
	sh := diagramSourceHash
	s3Path := h.putScenarioToS3(ctx, diagramVersionID, genYAML)
	overwrite := cached != nil
	out, err := h.scenarioCacheRepo.UpsertScenarioForDiagramVersion(ctx, diagramVersionID, genYAML, "generated", s3Path, &sh, overwrite)
	if err != nil {
		return "", diagramSourceHash, cached, err
	}
	return genYAML, diagramSourceHash, out, nil
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
	sourceHash := simrepo.HashAMGAPDSource(amgYAML)
	cached, err := h.scenarioCacheRepo.GetScenarioForDiagramVersion(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		serverScenarioError(c, "failed to load cached scenario", err)
		return
	}
	if cached != nil && cached.Source == "edited" && cached.SourceHash != "" && cached.SourceHash != sourceHash {
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
			"diagram_source_hash": sourceHash,
			"s3_path":             cached.S3Path,
			"updated_at":          cached.UpdatedAt,
		})
		return
	}
	if useCachedGeneratedScenario(cached, sourceHash) && cached != nil {
		sum := sha256.Sum256([]byte(cached.ScenarioYAML))
		c.JSON(http.StatusOK, gin.H{
			"scenario_yaml":       cached.ScenarioYAML,
			"scenario_hash":       hex.EncodeToString(sum[:]),
			"source":              cached.Source,
			"source_hash":         cached.SourceHash,
			"diagram_source_hash": sourceHash,
			"s3_path":             cached.S3Path,
			"updated_at":          cached.UpdatedAt,
		})
		return
	}
	genYAML, genErr := amg_apd_scenario.GenerateScenarioYAML([]byte(amgYAML))
	if genErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to generate valid scenario from AMG/APD YAML", "details": genErr.Error()})
		return
	}
	valRes, err := h.validateScenarioPreflight(c.Request.Context(), genYAML)
	if err != nil {
		h.writeScenarioValidationError(c, err, genYAML)
		return
	}
	sh := sourceHash
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
		"diagram_source_hash": sourceHash,
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
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(body.ScenarioYAML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario_yaml is required"})
		return
	}
	valRes, err := h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML)
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
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(body.ScenarioYAML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scenario_yaml is required"})
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
	valRes, err := h.validateScenarioPreflight(c.Request.Context(), body.ScenarioYAML)
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
	sourceHash := simrepo.HashAMGAPDSource(amgYAML)
	cached, err := h.scenarioCacheRepo.GetScenarioForDiagramVersion(c.Request.Context(), userID, projectID, diagramVersionID)
	if err != nil {
		serverScenarioError(c, "failed to load cached scenario", err)
		return
	}
	if cached != nil && cached.Source == "edited" && !body.Overwrite {
		c.JSON(http.StatusConflict, gin.H{"error": "an edited scenario exists; pass overwrite=true to replace it with a generated draft"})
		return
	}
	genYAML, genErr := amg_apd_scenario.GenerateScenarioYAML([]byte(amgYAML))
	if genErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to generate valid scenario from AMG/APD YAML", "details": genErr.Error()})
		return
	}
	valRes, err := h.validateScenarioPreflight(c.Request.Context(), genYAML)
	if err != nil {
		h.writeScenarioValidationError(c, err, genYAML)
		return
	}
	sh := sourceHash
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
		"diagram_source_hash": sourceHash,
		"s3_path":             saved.S3Path,
		"updated_at":          saved.UpdatedAt,
		"validation":          valRes,
	})
}

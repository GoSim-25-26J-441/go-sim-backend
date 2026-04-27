package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/domain"
	simrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/repository"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/realtime_system_simulation/service"
)

func newRuntimeTestHandler(t *testing.T, engineURL string) (*Handler, *service.SimulationService) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	simSvc := service.NewSimulationService(simrepo.NewRunRepository(rdb))
	h := &Handler{
		simService:   simSvc,
		engineClient: NewSimulationEngineClient(engineURL),
	}
	return h, simSvc
}

func createOwnedRunWithEngineID(t *testing.T, simSvc *service.SimulationService, userID, engineRunID string) string {
	t.Helper()
	created, err := simSvc.CreateRun(&domain.CreateRunRequest{UserID: userID})
	require.NoError(t, err)
	_, err = simSvc.UpdateRun(created.RunID, &domain.UpdateRunRequest{EngineRunID: &engineRunID})
	require.NoError(t, err)
	return created.RunID
}

func TestGetConfiguration_ProxiesEngineAndChecksOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/runs/eng-123/configuration", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"services":[{"id":"checkout","replicas":2}],"workload":[{"pattern_key":"client:checkout:/read","rate_rps":25}]}`))
	}))
	defer engine.Close()

	h, simSvc := newRuntimeTestHandler(t, engine.URL)
	router := gin.New()
	router.GET("/runs/:id/configuration", h.GetConfiguration)
	runID := createOwnedRunWithEngineID(t, simSvc, "user-1", "eng-123")

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/configuration", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "services")

	reqForbidden := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/configuration", nil)
	reqForbidden.Header.Set("X-User-Id", "user-2")
	wForbidden := httptest.NewRecorder()
	router.ServeHTTP(wForbidden, reqForbidden)
	require.Equal(t, http.StatusForbidden, wForbidden.Code)
}

func TestGetConfiguration_PreservesEngine404And412(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusNotFound, http.StatusPreconditionFailed} {
		t.Run("status_"+strconv.Itoa(status), func(t *testing.T) {
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/v1/runs/eng-404/configuration", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"engine-config-state"}`))
			}))
			defer engine.Close()
			h, simSvc := newRuntimeTestHandler(t, engine.URL)
			router := gin.New()
			router.GET("/runs/:id/configuration", h.GetConfiguration)
			runID := createOwnedRunWithEngineID(t, simSvc, "user-1", "eng-404")

			req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/configuration", nil)
			req.Header.Set("X-User-Id", "user-1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, status, w.Code)
			assert.Contains(t, w.Body.String(), "engine-config-state")
		})
	}
}

func TestRuntimeMutations_PreserveEngineStatusAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusPreconditionFailed} {
		t.Run("configuration_"+strconv.Itoa(status), func(t *testing.T) {
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch && r.URL.Path == "/v1/runs/eng-1/configuration" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"error":"engine-config-failure"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer engine.Close()
			h, simSvc := newRuntimeTestHandler(t, engine.URL)
			router := gin.New()
			router.PATCH("/runs/:id/configuration", h.UpdateConfiguration)
			runID := createOwnedRunWithEngineID(t, simSvc, "user-1", "eng-1")

			body := []byte(`{"services":[{"id":"checkout","replicas":3}]}`)
			req := httptest.NewRequest(http.MethodPatch, "/runs/"+runID+"/configuration", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Id", "user-1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, status, w.Code)
			assert.Contains(t, w.Body.String(), "engine-config-failure")
		})

		t.Run("workload_"+strconv.Itoa(status), func(t *testing.T) {
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch && r.URL.Path == "/v1/runs/eng-2/workload" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"error":"engine-workload-failure"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer engine.Close()
			h, simSvc := newRuntimeTestHandler(t, engine.URL)
			router := gin.New()
			router.PATCH("/runs/:id/workload", h.UpdateWorkload)
			runID := createOwnedRunWithEngineID(t, simSvc, "user-1", "eng-2")

			body := []byte(`{"pattern_key":"client:checkout:/read","rate_rps":25}`)
			req := httptest.NewRequest(http.MethodPatch, "/runs/"+runID+"/workload", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-Id", "user-1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, status, w.Code)
			assert.Contains(t, w.Body.String(), "engine-workload-failure")
		})
	}
}

package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/service"
)

func TestUpstreamClient_GetIntermediate(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/test-job/intermediate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"nodes": [], "edges": []}`))
	}))
	defer server.Close()

	client := service.NewUpstreamClient(server.URL)
	ctx := context.Background()

	resp, err := client.GetIntermediate(ctx, "test-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestUpstreamClient_GetIntermediate_Error(t *testing.T) {
	// Create a client with invalid URL
	client := service.NewUpstreamClient("http://invalid-url-that-does-not-exist")
	ctx := context.Background()

	resp, err := client.GetIntermediate(ctx, "test-job")
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatal("expected error, got nil")
	}
}

func TestUpstreamClient_Fuse(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/test-job/fuse" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-User-Id") != "test-user" {
			t.Errorf("expected X-User-Id header, got: %s", r.Header.Get("X-User-Id"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "fused"}`))
	}))
	defer server.Close()

	client := service.NewUpstreamClient(server.URL)
	ctx := context.Background()

	resp, err := client.Fuse(ctx, "test-job", "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestUpstreamClient_GetExportJSON(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/test-job/export" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"services": [], "dependencies": []}`))
	}))
	defer server.Close()

	client := service.NewUpstreamClient(server.URL)
	ctx := context.Background()

	spec, err := client.GetExportJSON(ctx, "test-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec == nil {
		t.Fatal("expected spec map, got nil")
	}
}

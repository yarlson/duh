package server

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yarlson/duh/docker"
	"github.com/yarlson/duh/service"
	"github.com/yarlson/duh/store"
)

//go:embed test_data
var testFiles embed.FS

func TestServerContainers(t *testing.T) {
	// Setup test service
	memoryStore := store.NewStore(time.Minute)
	mockDocker := &DockerClientMock{
		ListContainersFunc: func(ctx context.Context, all bool) ([]docker.Container, error) {
			return []docker.Container{
				{
					ID:     "test1",
					Names:  []string{"container1"},
					State:  "running",
					Status: "Up 2 hours",
				},
			}, nil
		},
		GetContainerStatsFunc: func(ctx context.Context, id string) (*docker.ContainerStats, error) {
			return &docker.ContainerStats{}, nil
		},
	}
	containerService := service.New(mockDocker, memoryStore)

	// Create server
	srv := New(containerService, testFiles)

	// Create test request
	req := httptest.NewRequest("GET", "/api/containers", nil)
	w := httptest.NewRecorder()

	// Handle request
	srv.handleContainers(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var containers []store.ContainerData
	if err := json.NewDecoder(w.Body).Decode(&containers); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(containers) != 0 { // Initially empty because we haven't synced
		t.Errorf("Expected empty container list, got %d containers", len(containers))
	}

	// Test sync
	if err := containerService.Sync(context.Background()); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Test again after sync
	w = httptest.NewRecorder()
	srv.handleContainers(w, req)

	if err := json.NewDecoder(w.Body).Decode(&containers); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(containers))
	}
}

func TestServerContainerActions(t *testing.T) {
	mockDocker := &DockerClientMock{
		StartContainerFunc: func(ctx context.Context, id string) error {
			return nil
		},
		StopContainerFunc: func(ctx context.Context, id string) error {
			return nil
		},
		ListContainersFunc: func(ctx context.Context, all bool) ([]docker.Container, error) {
			return []docker.Container{
				{
					ID:      "test",
					Names:   []string{"test-container"},
					Image:   "test-image",
					State:   "running",
					Status:  "Up 2 minutes",
					Created: time.Now().Unix(),
				},
			}, nil
		},
	}

	store := store.NewStore(time.Minute)
	service := service.New(mockDocker, store)
	server := New(service, testFiles)

	// Test start container
	req := httptest.NewRequest("POST", "/api/containers/test1?action=start", nil)
	w := httptest.NewRecorder()

	server.handleContainer(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
	}

	// Test stop container
	req = httptest.NewRequest("POST", "/api/containers/test1?action=stop", nil)
	w = httptest.NewRecorder()

	server.handleContainer(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status code %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify mock calls
	if len(mockDocker.StartContainerCalls()) != 1 {
		t.Error("Expected one call to StartContainer")
	}
	if len(mockDocker.StopContainerCalls()) != 1 {
		t.Error("Expected one call to StopContainer")
	}
}

func TestHandleContainers(t *testing.T) {
	// Create mock Docker client
	mockClient := &DockerClientMock{
		ListContainersFunc: func(ctx context.Context, all bool) ([]docker.Container, error) {
			return []docker.Container{
				{
					ID:      "123",
					Names:   []string{"test"},
					Image:   "test:latest",
					State:   "running",
					Status:  "Up 2 hours",
					Created: 1234567890,
				},
			}, nil
		},
		GetContainerStatsFunc: func(ctx context.Context, id string) (*docker.ContainerStats, error) {
			return &docker.ContainerStats{
				MemoryStats: struct {
					Usage uint64 `json:"usage"`
					Limit uint64 `json:"limit"`
				}{
					Usage: 1024 * 1024,      // 1MB
					Limit: 1024 * 1024 * 64, // 64MB
				},
			}, nil
		},
	}

	// Create store with a reasonable TTL
	memoryStore := store.NewStore(time.Minute)

	// Create service with mock client
	containerService := service.New(mockClient, memoryStore)

	// Sync the service first
	if err := containerService.Sync(context.Background()); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Give the store a moment to update
	time.Sleep(10 * time.Millisecond)

	// Verify store has been updated
	containers := memoryStore.List()
	if len(containers) != 1 {
		t.Fatalf("Store not updated, expected 1 container, got %d", len(containers))
	}

	// Create server with test files
	srv := New(containerService, testFiles)

	// Create test request
	req := httptest.NewRequest("GET", "/api/containers", nil)
	w := httptest.NewRecorder()

	// Handle request
	srv.handleContainers(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var responseContainers []store.ContainerData
	if err := json.NewDecoder(w.Body).Decode(&responseContainers); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(responseContainers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(responseContainers))
		return // Prevent index out of range panic
	}

	if responseContainers[0].ID != "123" {
		t.Errorf("Expected container ID '123', got '%s'", responseContainers[0].ID)
	}
}

func TestHandleContainer(t *testing.T) {
	// Create mock Docker client
	mockClient := &DockerClientMock{
		GetContainerStatsFunc: func(ctx context.Context, id string) (*docker.ContainerStats, error) {
			return &docker.ContainerStats{}, nil
		},
	}

	// Create service with mock client
	containerService := service.New(mockClient, store.NewStore(0))

	// Create server with test files
	srv := New(containerService, testFiles)

	// Create test request
	req := httptest.NewRequest("GET", "/api/containers/123", nil)
	w := httptest.NewRecorder()

	// Handle request
	srv.handleContainer(w, req)

	// Check response
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
	}
}

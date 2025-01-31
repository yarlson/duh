package service

import (
	"context"
	"testing"
	"time"

	"github.com/yarlson/duh/docker"
	"github.com/yarlson/duh/store"
)

func TestServiceSync(t *testing.T) {
	mockDocker := &DockerClientMock{
		ListContainersFunc: func(ctx context.Context, all bool) ([]docker.Container, error) {
			return []docker.Container{
				{
					ID:      "container1",
					Names:   []string{"test1"},
					Image:   "nginx",
					State:   "running",
					Status:  "Up 2 hours",
					Created: time.Now().Unix(),
				},
				{
					ID:      "container2",
					Names:   []string{"test2"},
					Image:   "redis",
					State:   "exited",
					Status:  "Exited (0) 1 hour ago",
					Created: time.Now().Unix(),
				},
			}, nil
		},
		GetContainerStatsFunc: func(ctx context.Context, id string) (*docker.ContainerStats, error) {
			return &docker.ContainerStats{
				CPUStats: struct {
					CPUUsage struct {
						TotalUsage uint64 `json:"total_usage"`
					} `json:"cpu_usage"`
					SystemCPUUsage uint64 `json:"system_cpu_usage"`
					OnlineCPUs     uint32 `json:"online_cpus"`
				}{
					CPUUsage: struct {
						TotalUsage uint64 `json:"total_usage"`
					}{
						TotalUsage: 100000000,
					},
					SystemCPUUsage: 1000000000,
					OnlineCPUs:     4,
				},
				PreCPUStats: struct {
					CPUUsage struct {
						TotalUsage uint64 `json:"total_usage"`
					} `json:"cpu_usage"`
					SystemCPUUsage uint64 `json:"system_cpu_usage"`
				}{
					CPUUsage: struct {
						TotalUsage uint64 `json:"total_usage"`
					}{
						TotalUsage: 90000000,
					},
					SystemCPUUsage: 990000000,
				},
				MemoryStats: struct {
					Usage uint64 `json:"usage"`
					Limit uint64 `json:"limit"`
				}{
					Usage: 104857600,  // 100MB
					Limit: 1073741824, // 1GB
				},
			}, nil
		},
	}

	memoryStore := store.NewStore(time.Minute)
	service := New(mockDocker, memoryStore)

	// Test Sync
	err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify containers were stored
	containers := service.List()
	if len(containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(containers))
	}

	// Verify running container has stats
	container1, exists := service.Get("container1")
	if !exists {
		t.Fatal("Container1 not found")
	}
	if container1.Stats == nil {
		t.Error("Expected stats for running container")
	}
	if container1.Stats.Memory.Usage != 104857600 {
		t.Errorf("Expected memory usage 104857600, got %d", container1.Stats.Memory.Usage)
	}

	// Verify non-running container has no stats
	container2, exists := service.Get("container2")
	if !exists {
		t.Fatal("Container2 not found")
	}
	if container2.Stats != nil {
		t.Error("Expected no stats for non-running container")
	}

	// Verify ListContainers was called once
	if len(mockDocker.ListContainersCalls()) != 1 {
		t.Error("Expected one call to ListContainers")
	}

	// Verify GetContainerStats was called only for running container
	statsCalls := mockDocker.GetContainerStatsCalls()
	if len(statsCalls) != 1 {
		t.Errorf("Expected one call to GetContainerStats, got %d", len(statsCalls))
	}
	if len(statsCalls) > 0 && statsCalls[0].ID != "container1" {
		t.Errorf("Expected GetContainerStats call for container1, got %s", statsCalls[0].ID)
	}
}

func TestServiceStartStop(t *testing.T) {
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
					ID:      "test-id",
					Names:   []string{"test-container"},
					Image:   "test-image",
					State:   "running",
					Status:  "Up 2 minutes",
					Created: time.Now().Unix(),
				},
			}, nil
		},
	}

	memoryStore := store.NewStore(time.Minute)
	service := New(mockDocker, memoryStore)

	// Test StartContainer
	err := service.StartContainer(context.Background(), "test-id")
	if err != nil {
		t.Errorf("StartContainer failed: %v", err)
	}
	if len(mockDocker.StartContainerCalls()) != 1 {
		t.Error("Expected one call to StartContainer")
	}

	// Verify container is in starting state
	container, exists := service.Get("test-id")
	if !exists {
		t.Fatal("Container not found after StartContainer")
	}
	if container.State != store.StateStarting {
		t.Errorf("Expected state %s, got %s", store.StateStarting, container.State)
	}

	// Test StopContainer
	err = service.StopContainer(context.Background(), "test-id")
	if err != nil {
		t.Errorf("StopContainer failed: %v", err)
	}
	if len(mockDocker.StopContainerCalls()) != 1 {
		t.Error("Expected one call to StopContainer")
	}

	// Verify container is in stopping state
	container, exists = service.Get("test-id")
	if !exists {
		t.Fatal("Container not found after StopContainer")
	}
	if container.State != store.StateStopping {
		t.Errorf("Expected state %s, got %s", store.StateStopping, container.State)
	}
}

func TestServiceStartStopStates(t *testing.T) {
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
					ID:     "test-id",
					State:  "running",
					Status: "Up 1 second",
				},
			}, nil
		},
	}

	memoryStore := store.NewStore(time.Minute)
	service := New(mockDocker, memoryStore)

	// Test StartContainer state transition
	err := service.StartContainer(context.Background(), "test-id")
	if err != nil {
		t.Errorf("StartContainer failed: %v", err)
	}

	// Verify intermediate state was set
	container, exists := service.Get("test-id")
	if !exists {
		t.Fatal("Container not found after StartContainer")
	}
	if container.State != store.StateStarting {
		t.Errorf("Expected state %s, got %s", store.StateStarting, container.State)
	}

	// Test StopContainer state transition
	err = service.StopContainer(context.Background(), "test-id")
	if err != nil {
		t.Errorf("StopContainer failed: %v", err)
	}

	// Verify intermediate state was set
	container, exists = service.Get("test-id")
	if !exists {
		t.Fatal("Container not found after StopContainer")
	}
	if container.State != store.StateStopping {
		t.Errorf("Expected state %s, got %s", store.StateStopping, container.State)
	}

	// Verify that Sync preserves intermediate states
	err = service.Sync(context.Background())
	if err != nil {
		t.Errorf("Sync failed: %v", err)
	}

	container, exists = service.Get("test-id")
	if !exists {
		t.Fatal("Container not found after Sync")
	}
	if container.State != store.StateStopping {
		t.Errorf("Expected state %s to be preserved after Sync, got %s",
			store.StateStopping, container.State)
	}
}

func TestSortContainers(t *testing.T) {
	now := time.Now().Unix()
	testCases := []struct {
		name     string
		input    []store.ContainerData
		expected []string // expected order of container IDs
	}{
		{
			name: "sort by status priority",
			input: []store.ContainerData{
				{
					ID:      "container1",
					State:   "exited",
					Created: now,
				},
				{
					ID:      "container2",
					State:   "running",
					Created: now - 100,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
				{
					ID:      "container3",
					State:   store.StateStopping,
					Created: now - 50,
				},
			},
			expected: []string{"container2", "container3", "container1"},
		},
		{
			name: "same status, sort by memory then creation time",
			input: []store.ContainerData{
				{
					ID:      "container1",
					State:   "running",
					Created: now,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
				{
					ID:      "container2",
					State:   "running",
					Created: now - 50,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 200,
						},
					},
				},
				{
					ID:      "container3",
					State:   "running",
					Created: now - 100,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 200,
						},
					},
				},
			},
			expected: []string{"container2", "container3", "container1"},
		},
		{
			name: "sort by memory usage desc",
			input: []store.ContainerData{
				{
					ID:      "container1",
					Created: now,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
				{
					ID:      "container2",
					Created: now,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 200,
						},
					},
				},
			},
			expected: []string{"container2", "container1"},
		},
		{
			name: "sort by creation time when memory is equal",
			input: []store.ContainerData{
				{
					ID:      "container1",
					Created: now - 100,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
				{
					ID:      "container2",
					Created: now,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
			},
			expected: []string{"container2", "container1"},
		},
		{
			name: "containers without stats come last",
			input: []store.ContainerData{
				{
					ID:      "container1",
					Created: now,
					Stats:   nil,
				},
				{
					ID:      "container2",
					Created: now - 100,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
			},
			expected: []string{"container2", "container1"},
		},
		{
			name: "mixed sorting scenario",
			input: []store.ContainerData{
				{
					ID:      "container1",
					Created: now - 100,
					Stats:   nil,
				},
				{
					ID:      "container2",
					Created: now,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 200,
						},
					},
				},
				{
					ID:      "container3",
					Created: now - 50,
					Stats: &store.Stats{
						Memory: struct {
							Usage uint64 `json:"usage"`
							Limit uint64 `json:"limit"`
						}{
							Usage: 100,
						},
					},
				},
			},
			expected: []string{"container2", "container3", "container1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sortContainers(tc.input)

			// Verify the order
			for i, expectedID := range tc.expected {
				if tc.input[i].ID != expectedID {
					t.Errorf("Expected container at position %d to be %s, got %s",
						i, expectedID, tc.input[i].ID)
				}
			}
		})
	}
}

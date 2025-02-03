package service

import (
	"context"
	"sort"
	"sync"

	"github.com/yarlson/duh/docker"
	"github.com/yarlson/duh/store"
)

//go:generate moq -out mock_docker_test.go . DockerClient

// DockerClient defines the interface for Docker operations
type DockerClient interface {
	ListContainers(ctx context.Context, all bool) ([]docker.Container, error)
	GetContainerStats(ctx context.Context, id string) (*docker.ContainerStats, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
}

// Store defines the interface for container data storage
type Store interface {
	Update(container store.ContainerData)
	UpdateStats(id string, stats *store.Stats) bool
	List() []store.ContainerData
	Get(id string) (store.ContainerData, bool)
	RemoveStaleData()
}

// ContainerService coordinates between Docker client and data store
type ContainerService struct {
	client DockerClient
	store  Store
}

// New creates a new container service
func New(client DockerClient, store Store) *ContainerService {
	return &ContainerService{
		client: client,
		store:  store,
	}
}

// SyncContainers updates the container list from Docker and handles state transitions.
// It returns the list of containers from Docker so that stats can be updated separately.
func (s *ContainerService) SyncContainers(ctx context.Context) ([]docker.Container, error) {
	// Get all containers from Docker.
	containers, err := s.client.ListContainers(ctx, true)
	if err != nil {
		return nil, err
	}

	// Create a map for quick container lookup.
	containerMap := make(map[string]docker.Container)
	for _, c := range containers {
		containerMap[c.ID] = c
	}

	// First, check all stored containers for state transitions.
	for _, stored := range s.store.List() {
		dockerC, exists := containerMap[stored.ID]
		switch stored.State {
		case store.StateStarting:
			if !exists || dockerC.State == "running" {
				if exists {
					s.store.Update(store.ContainerData{
						ID:      dockerC.ID,
						Names:   dockerC.Names,
						Image:   dockerC.Image,
						State:   dockerC.State,
						Status:  dockerC.Status,
						Created: dockerC.Created,
					})
				}
			}
		case store.StateStopping:
			if !exists || dockerC.State == "exited" {
				if exists {
					s.store.Update(store.ContainerData{
						ID:      dockerC.ID,
						Names:   dockerC.Names,
						Image:   dockerC.Image,
						State:   dockerC.State,
						Status:  dockerC.Status,
						Created: dockerC.Created,
					})
				}
			}
		}
	}

	// Then, update all containers that aren't in transition.
	for _, c := range containers {
		if stored, exists := s.store.Get(c.ID); exists {
			if stored.State == store.StateStarting || stored.State == store.StateStopping {
				continue // Skip containers in transition
			}
		}

		data := store.ContainerData{
			ID:      c.ID,
			Names:   c.Names,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			Created: c.Created,
		}
		s.store.Update(data)
	}
	return containers, nil
}

// SyncStats updates statistics for running containers.
// It accepts the container list (typically returned from SyncContainers) so that these operations are decoupled.
func (s *ContainerService) SyncStats(ctx context.Context, containers []docker.Container) {
	var wg sync.WaitGroup
	for _, c := range containers {
		// Update stats only for running containers.
		if c.State == "running" {
			wg.Add(1)
			go func(c docker.Container) {
				defer wg.Done()
				stats, err := s.client.GetContainerStats(ctx, c.ID)
				if err != nil {
					return // Skip stats on error
				}

				// Convert Docker stats to store stats.
				storeStats := &store.Stats{}
				storeStats.Memory.Usage = stats.MemoryStats.Usage
				storeStats.Memory.Limit = stats.MemoryStats.Limit

				// Calculate CPU percentage.
				cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
				systemDelta := stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage

				if systemDelta > 0 && cpuDelta > 0 {
					// Convert to nanoseconds for more precise calculation
					cpuDeltaNs := float64(cpuDelta)
					systemDeltaNs := float64(systemDelta)

					// Calculate CPU usage percentage per core
					numCPUs := float64(stats.CPUStats.OnlineCPUs)
					if numCPUs == 0 {
						numCPUs = 1 // fallback if OnlineCPUs is not reported
					}

					// Calculate CPU usage percentage
					// This gives us the percentage of CPU time this container used
					// across all cores during this interval
					cpuPercent := (cpuDeltaNs / systemDeltaNs) * 100.0

					// Scale to per-core percentage (e.g., 50% of 2 cores = 100%)
					cpuPercent *= numCPUs

					// Round to 2 decimal places for display
					cpuPercent = float64(int(cpuPercent*100)) / 100

					storeStats.CPU.Usage = cpuPercent
				}
				storeStats.CPU.Cores = stats.CPUStats.OnlineCPUs
				storeStats.CPU.SystemMS = stats.CPUStats.SystemCPUUsage / 1_000_000 // Convert to milliseconds

				s.store.UpdateStats(c.ID, storeStats)
			}(c)
		}
	}
	wg.Wait()
}

// Sync is updated to first sync the container list and then the statistics.
func (s *ContainerService) Sync(ctx context.Context) error {
	containers, err := s.SyncContainers(ctx)
	if err != nil {
		return err
	}
	s.SyncStats(ctx, containers)
	s.store.RemoveStaleData()
	return nil
}

// StartContainer starts a container and waits for it to be running
func (s *ContainerService) StartContainer(ctx context.Context, id string) error {
	// Get existing container data first
	existing, exists := s.store.Get(id)
	if !exists {
		// If container doesn't exist in store, try to get it from Docker
		containers, err := s.client.ListContainers(ctx, true)
		if err == nil {
			for _, c := range containers {
				if c.ID == id {
					existing = store.ContainerData{
						ID:      c.ID,
						Names:   c.Names,
						Image:   c.Image,
						Created: c.Created,
					}
					break
				}
			}
		}
	}

	// Set intermediate state while preserving other fields
	existing.State = store.StateStarting
	existing.Status = "Starting" // Add status to show in UI
	s.store.Update(existing)

	err := s.client.StartContainer(ctx, id)
	if err != nil {
		// On error, try to get current state from Docker
		containers, listErr := s.client.ListContainers(ctx, true)
		if listErr == nil {
			for _, c := range containers {
				if c.ID == id {
					existing.State = c.State
					existing.Status = c.Status
					s.store.Update(existing)
					break
				}
			}
		}
		return err
	}

	// Let the next Sync update pick up the final state
	return nil
}

// StopContainer stops a container and waits for it to exit
func (s *ContainerService) StopContainer(ctx context.Context, id string) error {
	// Get existing container data first
	existing, exists := s.store.Get(id)
	if !exists {
		// If container doesn't exist in store, try to get it from Docker
		containers, err := s.client.ListContainers(ctx, true)
		if err == nil {
			for _, c := range containers {
				if c.ID == id {
					existing = store.ContainerData{
						ID:      c.ID,
						Names:   c.Names,
						Image:   c.Image,
						Created: c.Created,
					}
					break
				}
			}
		}
	}

	// Set intermediate state while preserving other fields
	existing.State = store.StateStopping
	existing.Status = "Stopping" // Add status to show in UI
	s.store.Update(existing)

	// Send stop command
	err := s.client.StopContainer(ctx, id)
	if err != nil {
		// On error, try to get current state from Docker
		containers, listErr := s.client.ListContainers(ctx, true)
		if listErr == nil {
			for _, c := range containers {
				if c.ID == id {
					existing.State = c.State
					existing.Status = c.Status
					s.store.Update(existing)
					break
				}
			}
		}
		return err
	}

	// Let the next Sync update pick up the final state
	return nil
}

// sortContainers sorts containers by status (running > stopping > starting > exited),
// then by memory usage (desc), and finally by creation time (desc)
func sortContainers(containers []store.ContainerData) {
	sort.Slice(containers, func(i, j int) bool {
		// First sort by status priority
		iPriority := getStatusPriority(containers[i].State)
		jPriority := getStatusPriority(containers[j].State)
		if iPriority != jPriority {
			return iPriority < jPriority // Lower number = higher priority
		}

		// If status is the same, sort by memory usage
		// Only compare memory if both containers have stats
		if containers[i].Stats != nil && containers[j].Stats != nil {
			if containers[i].Stats.Memory.Usage != containers[j].Stats.Memory.Usage {
				return containers[i].Stats.Memory.Usage > containers[j].Stats.Memory.Usage
			}
		} else if containers[i].Stats != nil {
			return true // Container with stats comes first
		} else if containers[j].Stats != nil {
			return false // Container with stats comes first
		}

		// If memory usage is the same or no stats, sort by creation time desc
		return containers[i].Created > containers[j].Created
	})
}

// getStatusPriority returns a priority number for sorting container states
// Lower number = higher priority
func getStatusPriority(state string) int {
	switch state {
	case "running":
		return 0
	case store.StateStopping:
		return 1
	case store.StateStarting:
		return 2
	case "exited":
		return 3
	default:
		return 4
	}
}

// List returns all container data from the store, sorted by memory usage and creation time
func (s *ContainerService) List() []store.ContainerData {
	containers := s.store.List()
	sortContainers(containers)
	return containers
}

// Get returns container data by ID from the store
func (s *ContainerService) Get(id string) (store.ContainerData, bool) {
	return s.store.Get(id)
}

package service

import (
	"context"
	"sort"

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

// Sync updates container data and stats from Docker
func (s *ContainerService) Sync(ctx context.Context) error {
	// Get all containers
	containers, err := s.client.ListContainers(ctx, true)
	if err != nil {
		return err
	}

	// Create a map for quick container lookup
	containerMap := make(map[string]docker.Container)
	for _, c := range containers {
		containerMap[c.ID] = c
	}

	// First check all stored containers for state transitions
	for _, stored := range s.store.List() {
		dockerC, exists := containerMap[stored.ID]

		switch stored.State {
		case store.StateStarting:
			if !exists || dockerC.State == "running" {
				// Container has finished starting
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
				// Container has finished stopping
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

	// Then update all containers that aren't in transition
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

		// Get stats only for running containers
		if c.State == "running" {
			stats, err := s.client.GetContainerStats(ctx, c.ID)
			if err != nil {
				continue // Skip stats on error
			}

			// Convert Docker stats to store stats
			storeStats := &store.Stats{}
			storeStats.Memory.Usage = stats.MemoryStats.Usage
			storeStats.Memory.Limit = stats.MemoryStats.Limit

			// Calculate CPU percentage
			cpuDelta := stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage
			systemDelta := stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage

			if systemDelta > 0 && cpuDelta > 0 {
				storeStats.CPU.Usage = float64(cpuDelta) / float64(systemDelta) * 100.0 * float64(stats.CPUStats.OnlineCPUs)
			}
			storeStats.CPU.Cores = stats.CPUStats.OnlineCPUs
			storeStats.CPU.SystemMS = stats.CPUStats.SystemCPUUsage / 1_000_000 // Convert to milliseconds

			s.store.UpdateStats(c.ID, storeStats)
		}
	}

	// Clean up stale data
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

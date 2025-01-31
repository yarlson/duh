package store

import (
	"sync"
	"time"
)

// Add these constants at the top of the file
const (
	StateStarting = "starting"
	StateStopping = "stopping"
)

// ContainerData represents container information for frontend consumption
type ContainerData struct {
	ID      string    `json:"id"`
	Names   []string  `json:"names"`
	Image   string    `json:"image"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Created int64     `json:"created"`
	Stats   *Stats    `json:"stats,omitempty"`
	Updated time.Time `json:"-"` // internal field for TTL
}

// Stats represents container resource usage statistics for frontend display
type Stats struct {
	Memory struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	CPU struct {
		Usage    float64 `json:"usage"`     // Percentage (0-100)
		Cores    uint32  `json:"cores"`     // Number of CPU cores
		SystemMS uint64  `json:"system_ms"` // System CPU time in milliseconds
	} `json:"cpu_stats"`
}

// Store represents an in-memory store for container data
type Store struct {
	mu         sync.RWMutex
	containers map[string]ContainerData
	ttl        time.Duration
	done       chan struct{}
}

// NewStore creates a new store with the specified TTL for container data
func NewStore(ttl time.Duration) *Store {
	s := &Store{
		containers: make(map[string]ContainerData),
		ttl:        ttl,
		done:       make(chan struct{}),
	}
	return s
}

// Close stops the cleanup goroutine
func (s *Store) Close() {
	close(s.done)
}

// RemoveStaleData removes container data that hasn't been updated within TTL
func (s *Store) RemoveStaleData() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, container := range s.containers {
		if now.Sub(container.Updated) > s.ttl {
			delete(s.containers, id)
		}
	}
}

// Update adds or updates container data in the store
func (s *Store) Update(container ContainerData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve stats for all states except exited
	if existing, exists := s.containers[container.ID]; exists {
		if container.State != "exited" {
			container.Stats = existing.Stats
		}
	}

	container.Updated = time.Now()
	s.containers[container.ID] = container
}

// UpdateStats updates stats for a specific container
func (s *Store) UpdateStats(id string, stats *Stats) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if container, exists := s.containers[id]; exists {
		container.Stats = stats
		container.Updated = time.Now()
		s.containers[id] = container
		return true
	}

	return false
}

// List returns all non-stale container data
func (s *Store) List() []ContainerData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]ContainerData, 0, len(s.containers))

	for _, container := range s.containers {
		if now.Sub(container.Updated) <= s.ttl {
			result = append(result, container)
		}
	}

	return result
}

// Get returns container data by ID
func (s *Store) Get(id string) (ContainerData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	container, exists := s.containers[id]
	if !exists {
		return ContainerData{}, false
	}

	if time.Since(container.Updated) > s.ttl {
		return ContainerData{}, false
	}

	return container, true
}

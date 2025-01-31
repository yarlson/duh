package store

import (
	"testing"
	"time"
)

func TestStoreUpdate(t *testing.T) {
	store := NewStore(time.Minute)

	container := ContainerData{
		ID:     "123",
		Names:  []string{"test-container"},
		Image:  "test-image",
		State:  "running",
		Status: "Up 5 minutes",
	}

	store.Update(container)

	// Check if container was stored
	got, exists := store.Get("123")
	if !exists {
		t.Fatal("Container not found after update")
	}

	if got.ID != container.ID || got.Names[0] != container.Names[0] {
		t.Errorf("Got %+v, want %+v", got, container)
	}
}

func TestStoreUpdateStats(t *testing.T) {
	store := NewStore(time.Minute)

	// First add a container
	container := ContainerData{
		ID:    "123",
		Names: []string{"test-container"},
	}
	store.Update(container)

	// Update stats
	stats := &Stats{}
	stats.Memory.Usage = 1024 * 1024 // 1MB
	stats.Memory.Limit = 2048 * 1024 // 2MB
	stats.CPU.Usage = 25.5           // 25.5%
	stats.CPU.Cores = 4
	stats.CPU.SystemMS = 1000

	// Try updating stats for non-existent container
	if store.UpdateStats("456", stats) {
		t.Error("UpdateStats returned true for non-existent container")
	}

	// Update stats for existing container
	if !store.UpdateStats("123", stats) {
		t.Error("UpdateStats returned false for existing container")
	}

	// Verify stats were updated
	got, exists := store.Get("123")
	if !exists {
		t.Fatal("Container not found after stats update")
	}

	if got.Stats.Memory.Usage != stats.Memory.Usage {
		t.Errorf("Memory usage = %d, want %d", got.Stats.Memory.Usage, stats.Memory.Usage)
	}
	if got.Stats.CPU.Usage != stats.CPU.Usage {
		t.Errorf("CPU usage = %f, want %f", got.Stats.CPU.Usage, stats.CPU.Usage)
	}
}

func TestStoreList(t *testing.T) {
	store := NewStore(time.Minute)

	// Add some containers
	containers := []ContainerData{
		{ID: "1", Names: []string{"container-1"}},
		{ID: "2", Names: []string{"container-2"}},
		{ID: "3", Names: []string{"container-3"}},
	}

	for _, c := range containers {
		store.Update(c)
	}

	// List all containers
	got := store.List()
	if len(got) != len(containers) {
		t.Errorf("Got %d containers, want %d", len(got), len(containers))
	}

	// Verify each container is in the list
	found := make(map[string]bool)
	for _, c := range got {
		found[c.ID] = true
	}

	for _, c := range containers {
		if !found[c.ID] {
			t.Errorf("Container %s not found in list", c.ID)
		}
	}
}

func TestStoreTTL(t *testing.T) {
	// Create store with a short TTL
	ttl := 100 * time.Millisecond
	s := NewStore(ttl)

	// Add test data
	s.Update(ContainerData{
		ID:      "test",
		Updated: time.Now(),
	})

	// Verify data is present
	containers := s.List()
	if len(containers) != 1 {
		t.Errorf("Expected 1 container initially, got %d", len(containers))
	}

	// Wait for TTL to expire
	time.Sleep(ttl + 10*time.Millisecond)

	// Verify data is gone
	containers = s.List()
	if len(containers) != 0 {
		t.Errorf("List returned %d containers after TTL expired, want 0", len(containers))
	}
}

func TestStoreRemoveStaleData(t *testing.T) {
	store := NewStore(time.Millisecond)

	// Add some containers
	containers := []ContainerData{
		{ID: "1", Names: []string{"container-1"}},
		{ID: "2", Names: []string{"container-2"}},
	}

	for _, c := range containers {
		store.Update(c)
	}

	// Wait for TTL to expire
	time.Sleep(2 * time.Millisecond)

	// Remove stale data
	store.RemoveStaleData()

	// Verify containers were removed
	if list := store.List(); len(list) != 0 {
		t.Errorf("Got %d containers after cleanup, want 0", len(list))
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := NewStore(time.Minute)
	done := make(chan bool)

	// Start multiple goroutines updating and reading
	for i := 0; i < 10; i++ {
		go func(id string) {
			container := ContainerData{
				ID:    id,
				Names: []string{"container-" + id},
			}
			store.Update(container)
			store.Get(id)
			store.List()
			done <- true
		}(string(rune('A' + i)))
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state
	list := store.List()
	if len(list) != 10 {
		t.Errorf("Got %d containers, want 10", len(list))
	}
}

func TestStoreAddMoreContainers(t *testing.T) {
	store := NewStore(time.Minute)

	// Add some containers
	containers := []ContainerData{
		{ID: "1", Names: []string{"container-1"}},
		{ID: "2", Names: []string{"container-2"}},
	}

	for _, c := range containers {
		store.Update(c)
	}

	// Add more containers
	for i := 0; i < 10; i++ {
		store.Update(ContainerData{
			ID:      string(rune('A' + i)),
			Names:   []string{"container-" + string(rune('A'+i))},
			Updated: time.Now(),
		})
	}

	// Verify all containers are present
	got := store.List()
	if len(got) != len(containers)+10 {
		t.Errorf("Got %d containers, want %d", len(got), len(containers)+10)
	}
}

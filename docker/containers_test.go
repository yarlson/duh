package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func setupTestContainers(t *testing.T) func() {
	t.Helper()

	// Stop and remove all containers
	containers, err := exec.Command("docker", "ps", "-q").Output()
	if err != nil {
		t.Fatalf("Failed to list containers: %v", err)
		return nil
	}

	// If there are running containers, stop them
	if len(containers) > 0 {
		containerIDs := strings.Fields(string(containers))
		args := append([]string{"stop"}, containerIDs...)
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Logf("Failed to stop containers: %v, output: %s", err, out)
			// Continue anyway as containers might not exist
		}
	}

	// Remove all containers (including stopped ones)
	containers, err = exec.Command("docker", "ps", "-aq").Output()
	if err != nil {
		t.Fatalf("Failed to list all containers: %v", err)
		return nil
	}

	if len(containers) > 0 {
		containerIDs := strings.Fields(string(containers))
		args := append([]string{"rm", "-f"}, containerIDs...)
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("Failed to remove containers: %v, output: %s", err, out)
			return nil
		}
	}

	// Start test containers
	commands := [][]string{
		{"docker", "run", "-d", "--name", "test-busybox", "busybox", "sleep", "3600"},
		{"docker", "run", "-d", "--name", "test-nginx", "nginx:alpine"},
	}

	for _, cmd := range commands {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to start container: %v\nOutput: %s", err, out)
			return nil
		}
	}

	// Wait for containers to be fully started
	time.Sleep(2 * time.Second)

	// Return cleanup function
	return func() {
		_ = exec.Command("docker", "stop", "test-busybox", "test-nginx").Run()
		_ = exec.Command("docker", "rm", "test-busybox", "test-nginx").Run()
	}
}

func TestListContainers(t *testing.T) {
	cleanup := setupTestContainers(t)
	defer cleanup()

	client := NewClient()
	ctx := context.Background()

	containers, err := client.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	// Should have exactly 2 containers
	if len(containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(containers))
	}

	// Check if both containers are running
	runningCount := 0
	foundBusybox := false
	foundNginx := false

	for _, container := range containers {
		if container.State == "running" {
			runningCount++
		}
		for _, name := range container.Names {
			name = strings.TrimPrefix(name, "/")
			if name == "test-busybox" {
				foundBusybox = true
			}
			if name == "test-nginx" {
				foundNginx = true
			}
		}
	}

	if runningCount != 2 {
		t.Errorf("Expected 2 running containers, got %d", runningCount)
	}
	if !foundBusybox {
		t.Error("Busybox container not found")
	}
	if !foundNginx {
		t.Error("Nginx container not found")
	}
}

func TestGetContainerStats(t *testing.T) {
	cleanup := setupTestContainers(t)
	defer cleanup()

	client := NewClient()
	ctx := context.Background()

	// Get containers to find their IDs
	containers, err := client.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	// Get stats for each container
	for _, container := range containers {
		stats, err := client.GetContainerStats(ctx, container.ID)
		if err != nil {
			t.Errorf("GetContainerStats failed for container %s: %v", container.ID, err)
			continue
		}

		// Basic validation of stats
		if stats.CPUStats.CPUUsage.TotalUsage == 0 {
			t.Errorf("Expected non-zero CPU usage for container %s", container.ID)
		}
		if stats.MemoryStats.Usage == 0 {
			t.Errorf("Expected non-zero memory usage for container %s", container.ID)
		}
	}
}

func waitForContainerState(t *testing.T, client *Client, containerID, expectedState string) error {
	t.Helper()
	ctx := context.Background()

	// Try for up to 10 seconds, checking every 100ms
	for i := 0; i < 100; i++ {
		containers, err := client.ListContainers(ctx, true)
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}

		for _, c := range containers {
			if c.ID == containerID && c.State == expectedState {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("container %s did not reach state %s within timeout", containerID, expectedState)
}

func TestStartStopContainer(t *testing.T) {
	cleanup := setupTestContainers(t)
	defer cleanup()

	client := NewClient()
	ctx := context.Background()

	// Get containers
	containers, err := client.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	// Test with the first container
	container := containers[0]

	// Stop the container
	err = client.StopContainer(ctx, container.ID)
	if err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}

	// Wait for container to stop
	if err := waitForContainerState(t, client, container.ID, "exited"); err != nil {
		t.Fatalf("Waiting for container to stop: %v", err)
	}

	// Verify container is stopped
	containers, err = client.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	var stoppedContainer Container
	for _, c := range containers {
		if c.ID == container.ID {
			stoppedContainer = c
			break
		}
	}

	if stoppedContainer.State != "exited" {
		t.Errorf("Expected container state to be 'exited', got '%s'", stoppedContainer.State)
	}

	// Start the container again
	err = client.StartContainer(ctx, container.ID)
	if err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	// Wait for container to start
	if err := waitForContainerState(t, client, container.ID, "running"); err != nil {
		t.Fatalf("Waiting for container to start: %v", err)
	}

	// Verify container is running
	containers, err = client.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	var startedContainer Container
	for _, c := range containers {
		if c.ID == container.ID {
			startedContainer = c
			break
		}
	}

	if startedContainer.State != "running" {
		t.Errorf("Expected container state to be 'running', got '%s'", startedContainer.State)
	}
}

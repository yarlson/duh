package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// Container represents a Docker container
type Container struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

// ContainerStats represents container resource usage statistics
type ContainerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}

// Client represents a Docker API client
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Docker client
func NewClient() *Client {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

// ListContainers returns all Docker containers
func (c *Client) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	url := "http://docker/containers/json"
	if all {
		url += "?all=true"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return containers, nil
}

// GetContainerStats returns stats for a specific container
func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	url := fmt.Sprintf("http://docker/containers/%s/stats?stream=false", containerID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var stats ContainerStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Calculate CPU percentage
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)

	// Update CPU stats
	if systemDelta > 0 && cpuDelta > 0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = 1 // fallback if OnlineCPUs is not reported
		}
		stats.CPUStats.CPUUsage.TotalUsage = uint64((cpuDelta / systemDelta) * numCPUs * 100.0)
	}

	return &stats, nil
}

// StartContainer starts a Docker container
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	url := fmt.Sprintf("http://docker/containers/%s/start", containerID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// StopContainer stops a Docker container
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	url := fmt.Sprintf("http://docker/containers/%s/stop", containerID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

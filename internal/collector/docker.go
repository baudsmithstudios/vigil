package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	dockerAPIVersion = "v1.43"

	// Maximum response sizes to prevent unbounded memory allocation.
	maxContainerListBytes = 8 << 20 // 8 MB — container list can be large on busy hosts
	maxContainerStatBytes = 1 << 20 // 1 MB — single container stats payload
)

// dockerCollector reads container stats via the Docker Engine API over a Unix socket.
type dockerCollector struct {
	client   *http.Client
	socket   string
	warnOnce sync.Once // log permission/connection errors once, not every tick
}

func newDockerCollector(socket string) *dockerCollector {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", socket, 2*time.Second)
		},
	}
	return &dockerCollector{
		socket: socket,
		client: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
	}
}

// dockerContainer is the JSON shape returned by GET /containers/json.
type dockerContainer struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	State string   `json:"State"`
}

// dockerStats is the subset of GET /containers/{id}/stats we need.
type dockerStats struct {
	CPUStats    dockerCPUStats    `json:"cpu_stats"`
	PreCPUStats dockerCPUStats    `json:"precpu_stats"`
	MemoryStats dockerMemoryStats `json:"memory_stats"`
}

type dockerCPUStats struct {
	CPUUsage    dockerCPUUsage `json:"cpu_usage"`
	SystemUsage uint64         `json:"system_cpu_usage"`
	OnlineCPUs  int            `json:"online_cpus"`
}

type dockerCPUUsage struct {
	TotalUsage uint64 `json:"total_usage"`
}

type dockerMemoryStats struct {
	Usage uint64            `json:"usage"`
	Stats map[string]uint64 `json:"stats"`
	Limit uint64            `json:"limit"`
}

func (d *dockerCollector) collect() []ContainerSnapshot {
	containers, err := d.listContainers()
	if err != nil {
		d.warnOnce.Do(func() {
			log.Printf("docker: cannot reach socket %s: %v (container monitoring disabled)", d.socket, err)
		})
		return nil
	}

	snapshots := make([]ContainerSnapshot, 0, len(containers))
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue // skip containers with no name
		}
		name := c.Names[0]
		if strings.HasPrefix(name, "/") {
			name = name[1:]
		}
		if !isValidContainerID(c.ID) {
			continue // skip containers with suspicious IDs
		}
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		snap := ContainerSnapshot{
			Name:   name,
			ID:     shortID,
			Status: c.State,
		}

		// Only fetch stats for running containers.
		if c.State == "running" {
			if stats, err := d.getStats(c.ID); err == nil {
				snap.CPUPercent = calculateCPUPercent(stats)
				snap.MemUsed = stats.MemoryStats.Usage
				// Subtract inactive_file (cache) from usage if available.
				if cache, ok := stats.MemoryStats.Stats["inactive_file"]; ok && snap.MemUsed > cache {
					snap.MemUsed -= cache
				}
				snap.MemLimit = stats.MemoryStats.Limit
				if snap.MemLimit > 0 {
					snap.MemPercent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100
				}
			}
		}

		snapshots = append(snapshots, snap)
	}
	return snapshots
}

// isValidContainerID checks that a container ID contains only hex characters.
var validContainerID = regexp.MustCompile(`^[a-f0-9]+$`)

func isValidContainerID(id string) bool {
	return len(id) > 0 && validContainerID.MatchString(id)
}

func (d *dockerCollector) listContainers() ([]dockerContainer, error) {
	resp, err := d.client.Get("http://localhost/" + dockerAPIVersion + "/containers/json?all=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker list: status %d", resp.StatusCode)
	}
	var containers []dockerContainer
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxContainerListBytes)).Decode(&containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func (d *dockerCollector) getStats(containerID string) (*dockerStats, error) {
	statsURL := fmt.Sprintf("http://localhost/%s/containers/%s/stats?stream=false&one-shot=true", dockerAPIVersion, url.PathEscape(containerID))
	resp, err := d.client.Get(statsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxContainerStatBytes))
	if err != nil {
		return nil, err
	}
	var stats dockerStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// calculateCPUPercent computes CPU % from the delta between precpu and cpu stats.
func calculateCPUPercent(stats *dockerStats) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if systemDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	cpus := stats.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = 1
	}
	return (cpuDelta / systemDelta) * float64(cpus) * 100.0
}

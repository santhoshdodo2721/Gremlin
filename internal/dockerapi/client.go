package dockerapi

import (
"bytes"
"context"
"encoding/json"
"fmt"
"io"
"net"
"net/http"
"time"
)

const apiVersion = "v1.43"

type Client struct {
http *http.Client
}

func New(socketPath string) *Client {
transport := &http.Transport{
DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
d := net.Dialer{}
return d.DialContext(ctx, "unix", socketPath)
},
}
return &Client{
http: &http.Client{
Transport: transport,
Timeout:   30 * time.Second,
},
}
}

func (c *Client) url(path string) string {
return fmt.Sprintf("http://docker/%s%s", apiVersion, path)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
req, err := http.NewRequestWithContext(ctx, method, c.url(path), body)
if err != nil {
return nil, err
}
if body != nil {
req.Header.Set("Content-Type", "application/json")
}
resp, err := c.http.Do(req)
if err != nil {
return nil, fmt.Errorf("docker daemon unreachable (is docker.sock mounted?): %w", err)
}
return resp, nil
}

type Container struct {
ID     string            `json:"Id"`
Names  []string          `json:"Names"`
Image  string            `json:"Image"`
State  string            `json:"State"`
Status string            `json:"Status"`
Labels map[string]string `json:"Labels"`
}

func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
resp, err := c.do(ctx, http.MethodGet, "/containers/json", nil)
if err != nil {
return nil, err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusOK {
return nil, fmt.Errorf("list containers: unexpected status %d", resp.StatusCode)
}
var containers []Container
if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
return nil, err
}
return containers, nil
}

func (c *Client) InspectContainer(ctx context.Context, id string) (map[string]any, error) {
resp, err := c.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil)
if err != nil {
return nil, err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusOK {
return nil, fmt.Errorf("inspect %s: unexpected status %d", id, resp.StatusCode)
}
var out map[string]any
if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
return nil, err
}
return out, nil
}

func (c *Client) StopContainer(ctx context.Context, id string, timeoutSeconds int) error {
path := fmt.Sprintf("/containers/%s/stop?t=%d", id, timeoutSeconds)
resp, err := c.do(ctx, http.MethodPost, path, nil)
if err != nil {
return err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
return fmt.Errorf("stop %s: unexpected status %d", id, resp.StatusCode)
}
return nil
}

func (c *Client) RestartContainer(ctx context.Context, id string, timeoutSeconds int) error {
path := fmt.Sprintf("/containers/%s/restart?t=%d", id, timeoutSeconds)
resp, err := c.do(ctx, http.MethodPost, path, nil)
if err != nil {
return err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusNoContent {
return fmt.Errorf("restart %s: unexpected status %d", id, resp.StatusCode)
}
return nil
}

func (c *Client) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
createBody, _ := json.Marshal(map[string]any{
"Cmd":          cmd,
"AttachStdout": true,
"AttachStderr": true,
})
resp, err := c.do(ctx, http.MethodPost, "/containers/"+containerID+"/exec", bytes.NewReader(createBody))
if err != nil {
return "", err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusCreated {
body, _ := io.ReadAll(resp.Body)
return "", fmt.Errorf("exec create: status %d: %s", resp.StatusCode, string(body))
}
var created struct {
Id string `json:"Id"`
}
if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
return "", err
}

startBody, _ := json.Marshal(map[string]any{"Detach": false, "Tty": false})
startResp, err := c.do(ctx, http.MethodPost, "/exec/"+created.Id+"/start", bytes.NewReader(startBody))
if err != nil {
return "", err
}
defer startResp.Body.Close()
out, err := io.ReadAll(startResp.Body)
if err != nil {
return "", err
}
return string(out), nil
}

// StartBackground runs cmd inside a container without waiting for it to
// finish - used for attacks that need to run *while* we poll health in
// parallel, rather than attacks like Exec() that block until completion.
func (c *Client) StartBackground(ctx context.Context, containerID string, cmd []string) error {
createBody, _ := json.Marshal(map[string]any{
"Cmd":          cmd,
"AttachStdout": false,
"AttachStderr": false,
})
resp, err := c.do(ctx, http.MethodPost, "/containers/"+containerID+"/exec", bytes.NewReader(createBody))
if err != nil {
return err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusCreated {
body, _ := io.ReadAll(resp.Body)
return fmt.Errorf("exec create: status %d: %s", resp.StatusCode, string(body))
}
var created struct {
Id string `json:"Id"`
}
if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
return err
}

startBody, _ := json.Marshal(map[string]any{"Detach": true, "Tty": false})
startResp, err := c.do(ctx, http.MethodPost, "/exec/"+created.Id+"/start", bytes.NewReader(startBody))
if err != nil {
return err
}
defer startResp.Body.Close()
return nil
}

// Stats is a single snapshot of a container's resource usage, taken
// directly from Docker's stats API (non-streaming, one-shot read).
type Stats struct {
CPUPercent float64
MemUsedMB  float64
MemLimitMB float64
}

// GetStats fetches one resource usage snapshot for a container by reading
// the Docker Engine's /stats endpoint with streaming disabled.
func (c *Client) GetStats(ctx context.Context, containerID string) (Stats, error) {
resp, err := c.do(ctx, http.MethodGet, "/containers/"+containerID+"/stats?stream=false", nil)
if err != nil {
return Stats{}, err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusOK {
body, _ := io.ReadAll(resp.Body)
return Stats{}, fmt.Errorf("stats: status %d: %s", resp.StatusCode, string(body))
}

var raw struct {
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
if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
return Stats{}, err
}

cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
sysDelta := float64(raw.CPUStats.SystemCPUUsage) - float64(raw.PreCPUStats.SystemCPUUsage)
onlineCPUs := float64(raw.CPUStats.OnlineCPUs)
if onlineCPUs == 0 {
onlineCPUs = 1
}
cpuPercent := 0.0
if sysDelta > 0 && cpuDelta > 0 {
cpuPercent = (cpuDelta / sysDelta) * onlineCPUs * 100.0
}

return Stats{
CPUPercent: cpuPercent,
MemUsedMB:  float64(raw.MemoryStats.Usage) / 1024.0 / 1024.0,
MemLimitMB: float64(raw.MemoryStats.Limit) / 1024.0 / 1024.0,
}, nil
}

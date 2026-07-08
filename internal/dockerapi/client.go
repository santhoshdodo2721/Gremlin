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

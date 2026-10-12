// Package client is a small Go client for the Agent Gateway.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/izhw/ai-agent/contract"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
	TenantID   string
	ProductID  string
	UserID     string
}

type RunStream struct {
	response *http.Response
	scanner  *bufio.Scanner
	closed   bool
}

func (c *Client) Start(
	ctx context.Context,
	agentKey, message, conversationID string,
) (contract.Run, *RunStream, error) {
	payload := map[string]any{"message": message}
	if conversationID != "" {
		payload["conversation_id"] = conversationID
	}
	var run contract.Run
	stream, err := c.doSSE(ctx, http.MethodPost, "/v1/agents/"+agentKey+"/runs", payload)
	if err != nil {
		return run, nil, err
	}
	if stream.response.Header.Get("X-Run-ID") != "" {
		run.ID = stream.response.Header.Get("X-Run-ID")
	}
	return run, stream, nil
}

func (c *Client) Resume(
	ctx context.Context,
	runID string,
	targets map[string]any,
) (*RunStream, error) {
	return c.doSSE(
		ctx,
		http.MethodPost,
		"/v1/runs/"+runID+"/resume",
		map[string]any{"targets": targets},
	)
}

func (c *Client) GetRun(ctx context.Context, runID string) (contract.Run, error) {
	request, err := c.request(ctx, http.MethodGet, "/v1/runs/"+runID, nil, "application/json")
	if err != nil {
		return contract.Run{}, err
	}
	defer request.Body.Close()
	var run contract.Run
	if err = json.NewDecoder(request.Body).Decode(&run); err != nil {
		return run, err
	}
	return run, nil
}

func (c *Client) doSSE(ctx context.Context, method, path string, body any) (*RunStream, error) {
	//nolint:bodyclose // RunStream owns and closes the response body.
	request, err := c.request(ctx, method, path, body, "text/event-stream")
	if err != nil {
		return nil, err
	}
	return &RunStream{response: request, scanner: bufio.NewScanner(request.Body)}, nil
}

func (c *Client) request(
	ctx context.Context,
	method, path string,
	body any,
	accept string,
) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(data))
	}
	url := strings.TrimRight(c.BaseURL, "/") + path
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.TenantID != "" {
		request.Header.Set("X-Tenant-ID", c.TenantID)
	}
	if c.ProductID != "" {
		request.Header.Set("X-Product-ID", c.ProductID)
	}
	if c.UserID != "" {
		request.Header.Set("X-User-ID", c.UserID)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if readErr != nil {
			return nil, fmt.Errorf(
				"agent gateway %s: read response body: %w",
				response.Status,
				readErr,
			)
		}
		return nil, fmt.Errorf("agent gateway %s: %s", response.Status, data)
	}
	return response, nil
}

func (s *RunStream) Next() (contract.RunEvent, error) {
	if s.closed {
		return contract.RunEvent{}, io.EOF
	}
	var eventType string
	var id string
	var data []string
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if line == "" {
			if len(data) == 0 {
				continue
			}
			var event contract.RunEvent
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event); err != nil {
				return event, err
			}
			if event.Type == "" && eventType != "" {
				event.Type = contract.EventType(eventType)
			}
			if event.ID == "" {
				event.ID = id
			}
			return event, nil
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "id:") {
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := s.scanner.Err(); err != nil {
		return contract.RunEvent{}, err
	}
	return contract.RunEvent{}, io.EOF
}

func (s *RunStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.response.Body.Close()
}

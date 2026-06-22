package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

const BaseURL = "https://status.openai.com"

func NewClient(timeout time.Duration) *Client {
	return newClient(BaseURL, timeout)
}

func newClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) FetchSummary(ctx context.Context) (Summary, error) {
	var summary Summary
	if err := c.getJSON(ctx, "/api/v2/summary.json", &summary); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (c *Client) FetchIncidents(ctx context.Context) (IncidentsResponse, error) {
	var incidents IncidentsResponse
	if err := c.getJSON(ctx, "/api/v2/incidents.json", &incidents); err != nil {
		return IncidentsResponse{}, err
	}
	return incidents, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "openai-status-bot/1.0")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("openai status API returned %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("decode openai status response: %w", err)
	}
	return nil
}

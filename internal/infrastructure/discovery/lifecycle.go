package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (c *Client) Register(ctx context.Context) error {
	body, err := json.Marshal(registrationPayload{Instance: c.instance})
	if err != nil {
		return fmt.Errorf("encoding eureka registration payload: %w", err)
	}

	url := fmt.Sprintf("%s/apps/%s", c.baseURL, c.appName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building eureka registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling eureka registration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("eureka registration returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Heartbeat(ctx context.Context) error {
	url := fmt.Sprintf("%s/apps/%s/%s", c.baseURL, c.appName, c.instanceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return fmt.Errorf("building eureka heartbeat request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling eureka heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eureka heartbeat returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Deregister(ctx context.Context) error {
	url := fmt.Sprintf("%s/apps/%s/%s", c.baseURL, c.appName, c.instanceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("building eureka deregistration request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling eureka deregistration: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

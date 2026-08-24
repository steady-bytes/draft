package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// slackWebhookPayload is the JSON shape a Slack incoming webhook expects:
// https://api.slack.com/messaging/webhooks.
type slackWebhookPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// postToSlack POSTs payload to webhookURL and returns the response body.
// A non-2xx status is reported as an error (its body included, for
// diagnostics), matching how Execute in rpc.go turns any such failure into
// an ordinary StepResponse{Success: false, Error: ...} rather than a Go
// error.
func postToSlack(ctx context.Context, client *http.Client, webhookURL string, payload slackWebhookPayload) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build slack webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call slack webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read slack webhook response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, fmt.Errorf("slack webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

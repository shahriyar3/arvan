package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

type sendRequestBody struct {
	MessageID string `json:"message_id"`
	AccountID string `json:"account_id"`
	To        string `json:"to"`
	Body      string `json:"body"`
}

type sendResponseBody struct {
	OperatorRef string `json:"operator_ref"`
}

func (c *HTTPClient) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	body, err := json.Marshal(sendRequestBody{
		MessageID: req.MessageID,
		AccountID: req.AccountID,
		To:        req.To,
		Body:      req.Body,
	})
	if err != nil {
		return SendResult{}, fmt.Errorf("marshal operator request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/sms", bytes.NewReader(body))
	if err != nil {
		return SendResult{}, fmt.Errorf("create operator request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return SendResult{}, fmt.Errorf("call operator: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return SendResult{}, fmt.Errorf("read operator response: %w", err)
	}

	if resp.StatusCode >= 500 {
		return SendResult{}, fmt.Errorf("operator unavailable: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 400 {
		return SendResult{}, &PermanentError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var parsed sendResponseBody
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return SendResult{}, fmt.Errorf("decode operator response: %w", err)
	}

	return SendResult{OperatorRef: parsed.OperatorRef}, nil
}

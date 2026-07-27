package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientSendSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/sms", r.URL.Path)

		var body sendRequestBody
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "msg-1", body.MessageID)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(sendResponseBody{OperatorRef: "op-ref-1"}))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, 2*time.Second)
	result, err := client.Send(context.Background(), SendRequest{
		MessageID: "msg-1",
		AccountID: "acc-1",
		To:        "+989121234567",
		Body:      "Hello",
	})
	require.NoError(t, err)
	assert.Equal(t, "op-ref-1", result.OperatorRef)
}

func TestHTTPClientSendOperatorFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, 2*time.Second)
	_, err := client.Send(context.Background(), SendRequest{
		MessageID: "msg-1",
		AccountID: "acc-1",
		To:        "+989121234567",
		Body:      "Hello",
	})
	require.Error(t, err)
	assert.False(t, IsPermanent(err))
	assert.Contains(t, err.Error(), "operator unavailable")
}

func TestHTTPClientSendOperatorRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, 2*time.Second)
	_, err := client.Send(context.Background(), SendRequest{
		MessageID: "msg-1",
		AccountID: "acc-1",
		To:        "+989121234567",
		Body:      "Hello",
	})
	require.Error(t, err)
	assert.True(t, IsPermanent(err))
}

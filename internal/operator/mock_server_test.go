package operator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockServerIdempotentByMessageID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := NewMockServer(config.MockOperatorConfig{})
	router := gin.New()
	server.Register(router)

	body := mockSendRequest{
		MessageID: "550e8400-e29b-41d4-a716-446655440000",
		AccountID: "660e8400-e29b-41d4-a716-446655440001",
		To:        "+989121234567",
		Body:      "Hello",
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	first := httptest.NewRecorder()
	firstReq, err := http.NewRequest(http.MethodPost, "/v1/sms", bytes.NewReader(raw))
	require.NoError(t, err)
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(first, firstReq)
	require.Equal(t, http.StatusOK, first.Code)

	var firstResp mockSendResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.NotEmpty(t, firstResp.OperatorRef)

	second := httptest.NewRecorder()
	secondReq, err := http.NewRequest(http.MethodPost, "/v1/sms", bytes.NewReader(raw))
	require.NoError(t, err)
	secondReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(second, secondReq)
	require.Equal(t, http.StatusOK, second.Code)

	var secondResp mockSendResponse
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))
	assert.Equal(t, firstResp.OperatorRef, secondResp.OperatorRef)
}

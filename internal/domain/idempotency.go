package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type IdempotencyRecord struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	Key              string
	ResponseSnapshot []byte
	CreatedAt        time.Time
}

type IdempotencyResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

func ParseIdempotencyResponse(snapshot []byte) (IdempotencyResponse, bool) {
	if len(snapshot) == 0 || string(snapshot) == "{}" {
		return IdempotencyResponse{}, false
	}

	var resp IdempotencyResponse
	if err := json.Unmarshal(snapshot, &resp); err != nil {
		return IdempotencyResponse{}, false
	}
	if resp.MessageID == "" {
		return IdempotencyResponse{}, false
	}

	return resp, true
}

func MarshalIdempotencyResponse(result SendSMSResult) ([]byte, error) {
	return json.Marshal(IdempotencyResponse{
		MessageID: result.MessageID.String(),
		Status:    result.Status,
	})
}

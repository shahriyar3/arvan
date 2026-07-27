package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIdempotencyResponse(t *testing.T) {
	messageID := uuid.New()

	t.Run("parses complete snapshot", func(t *testing.T) {
		snapshot, err := MarshalIdempotencyResponse(SendSMSResult{
			MessageID: messageID,
			Status:    SMSStatusAccepted,
		})
		require.NoError(t, err)

		resp, ok := ParseIdempotencyResponse(snapshot)
		require.True(t, ok)
		assert.Equal(t, messageID.String(), resp.MessageID)
		assert.Equal(t, SMSStatusAccepted, resp.Status)
	})

	t.Run("rejects placeholder snapshot", func(t *testing.T) {
		_, ok := ParseIdempotencyResponse([]byte("{}"))
		assert.False(t, ok)
	})

	t.Run("rejects empty snapshot", func(t *testing.T) {
		_, ok := ParseIdempotencyResponse(nil)
		assert.False(t, ok)
	})
}

package domain

import (
	"strings"
	"testing"

	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateE164(t *testing.T) {
	assert.True(t, ValidateE164("+989121234567"))
	assert.True(t, ValidateE164("+14155552671"))
	assert.False(t, ValidateE164("989121234567"))
	assert.False(t, ValidateE164("+0123456789"))
	assert.False(t, ValidateE164("+98912"))
	assert.False(t, ValidateE164(""))
}

func TestDetectEncoding(t *testing.T) {
	assert.Equal(t, EncodingGSM7, DetectEncoding("Hello world"))
	assert.Equal(t, EncodingUCS2, DetectEncoding("سلام"))
	assert.Equal(t, EncodingUCS2, DetectEncoding("Hello €"))
}

func TestValidateSinglePageBody(t *testing.T) {
	t.Run("accepts gsm7 within limit", func(t *testing.T) {
		body := strings.Repeat("a", GSM7MaxLength)
		encoding, err := ValidateSinglePageBody(body)
		require.NoError(t, err)
		assert.Equal(t, EncodingGSM7, encoding)
	})

	t.Run("rejects gsm7 over limit", func(t *testing.T) {
		body := strings.Repeat("a", GSM7MaxLength+1)
		_, err := ValidateSinglePageBody(body)
		assert.ErrorIs(t, err, domainerrors.ErrMessageTooLong)
	})

	t.Run("accepts ucs2 within limit", func(t *testing.T) {
		body := strings.Repeat("س", UCS2MaxLength)
		encoding, err := ValidateSinglePageBody(body)
		require.NoError(t, err)
		assert.Equal(t, EncodingUCS2, encoding)
	})

	t.Run("rejects ucs2 over limit", func(t *testing.T) {
		body := strings.Repeat("س", UCS2MaxLength+1)
		_, err := ValidateSinglePageBody(body)
		assert.ErrorIs(t, err, domainerrors.ErrMessageTooLong)
	})
}

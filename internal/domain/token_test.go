package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashTokenIsDeterministic(t *testing.T) {
	token := "demo-token-account-a"
	assert.Equal(t, HashToken(token), HashToken(token))
	assert.NotEqual(t, HashToken(token), HashToken("other-token"))
}

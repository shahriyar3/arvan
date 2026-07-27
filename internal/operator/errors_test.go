package operator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPermanent(t *testing.T) {
	assert.True(t, IsPermanent(&PermanentError{StatusCode: 400, Body: "bad request"}))
	assert.False(t, IsPermanent(errors.New("operator unavailable")))
	assert.False(t, IsPermanent(nil))
}

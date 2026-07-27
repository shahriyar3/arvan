package operator

import (
	"errors"
	"fmt"
)

type PermanentError struct {
	StatusCode int
	Body       string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("operator rejected request: status=%d body=%s", e.StatusCode, e.Body)
}

func IsPermanent(err error) bool {
	var target *PermanentError
	return errors.As(err, &target)
}

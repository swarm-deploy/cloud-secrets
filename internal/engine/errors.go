package engine

import "fmt"

type ErrSecretNotFound struct {
	ID string
}

func (e *ErrSecretNotFound) Error() string {
	return fmt.Sprintf("secret %q not found", e.ID)
}

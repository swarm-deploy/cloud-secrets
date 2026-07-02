package engine

import "fmt"

type SecretNotFoundError struct {
	ID string
}

func (e *SecretNotFoundError) Error() string {
	return fmt.Sprintf("secret %q not found", e.ID)
}

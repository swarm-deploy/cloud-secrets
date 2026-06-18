package secretname

import "fmt"

type FolderDelimiter string

const (
	FolderDelimiterDash       = "-"
	FolderDelimiterUnderscore = "_"
)

func (d FolderDelimiter) Validate() error {
	switch d {
	case FolderDelimiterDash, FolderDelimiterUnderscore:
		return nil
	default:
		return fmt.Errorf("invalid folder delimiter: %s, possible: [%s, %s]",
			d,
			FolderDelimiterDash,
			FolderDelimiterUnderscore,
		)
	}
}

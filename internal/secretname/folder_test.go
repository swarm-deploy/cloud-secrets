package secretname

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFolderDelimiterValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		delimiter FolderDelimiter
		wantErr   bool
	}{
		{
			name:      "dash is valid",
			delimiter: FolderDelimiterDash,
		},
		{
			name:      "underscore is valid",
			delimiter: FolderDelimiterUnderscore,
		},
		{
			name:      "custom delimiter is invalid",
			delimiter: ".",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.delimiter.Validate()
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

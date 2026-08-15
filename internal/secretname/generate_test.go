package secretname

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		path            string
		delimiter       FolderDelimiter
		versionID       string
		want            string
		wantUUIDOnError bool
	}{
		{
			name:      "replaces folders with delimiter and appends version",
			path:      "prod/db/password",
			delimiter: FolderDelimiterDash,
			versionID: "version-1",
			want:      "prod-db-password-version-1",
		},
		{
			name:      "keeps max length boundary",
			path:      strings.Repeat("a", MaxLength-len("version-1")-1),
			delimiter: FolderDelimiterUnderscore,
			versionID: "version-1",
			want:      strings.Repeat("a", MaxLength-len("version-1")-1) + "-version-1",
		},
		{
			name:            "returns uuid when max length exceeded",
			path:            strings.Repeat("a", MaxLength-len("version-1")),
			delimiter:       FolderDelimiterDash,
			versionID:       "version-1",
			wantUUIDOnError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Generate(tt.path, tt.delimiter, tt.versionID)

			if tt.wantUUIDOnError {
				_, err := uuid.Parse(got)
				assert.NoError(t, err)

				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

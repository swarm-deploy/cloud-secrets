package secretname

import (
	"strings"

	"github.com/google/uuid"
)

const MaxLength = 64

func Generate(
	path string,
	folderDelimiter FolderDelimiter,
	versionID string,
) string {
	if len(path)+len(versionID)+1 > MaxLength {
		return uuid.NewString()
	}

	return strings.ReplaceAll(path, "/", string(folderDelimiter)) + "-" + versionID
}

package cloudru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvider_Definition(t *testing.T) {
	t.Parallel()

	provider := &Provider{cfg: Config{ProjectID: "project-id"}}

	definition := provider.Definition()

	assert.Equal(t, "Cloud.ru Secret Manager", definition.Name)
	assert.Empty(t, definition.Links.Doc)
	assert.Equal(
		t,
		"https://console.cloud.ru/spa/secret-manager/list?projectId=project-id",
		definition.Links.Manager,
	)
}

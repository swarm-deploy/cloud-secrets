package vault

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvider_Definition(t *testing.T) {
	t.Parallel()

	provider := &Provider{cfg: Config{Addr: url.URL{
		Scheme: "https",
		Host:   "vault.example.com",
	}}}

	definition := provider.Definition()

	assert.Equal(t, "HashiCorp Vault", definition.Name)
	assert.Equal(t, "https://developer.hashicorp.com/vault/docs", definition.Links.Doc)
	assert.Empty(t, definition.Links.Manager)
}

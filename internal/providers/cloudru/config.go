package cloudru

import (
	"errors"
	"strings"
)

type RootFolder string

func (f *RootFolder) UnmarshalText(text []byte) error {
	*f = RootFolder(strings.Trim(string(text), "/"))

	return nil
}

type Config struct {
	// IAM configures shared authentication for both Cloud.ru sources.
	IAM struct {
		// Address optionally overrides the discovered gRPC endpoint.
		Address string `env:"ADDRESS"`

		// ClientID is loaded from the mounted file configured by CLOUDRU_IAM_CLIENT_ID.
		ClientID string `env:"CLIENT_ID,file,required,notEmpty"`
		// ClientSecret is loaded from the mounted file configured by CLOUDRU_IAM_CLIENT_SECRET.
		ClientSecret string `env:"CLIENT_SECRET,file,required,notEmpty" json:"-"`
	} `envPrefix:"IAM_"`
	// CSM configures the existing Secret Manager source.
	CSM struct {
		// Address optionally overrides the discovered gRPC endpoint.
		Address string `env:"ADDRESS"`
	} `envPrefix:"CSM_"`

	// CertificateManager configures the optional certificate source.
	CertificateManager struct {
		// Enabled opts into synchronizing all usable project certificates.
		Enabled bool `env:"ENABLED"`
		// Address optionally overrides the discovered/public HTTP endpoint.
		// Address optionally overrides the discovered gRPC endpoint.
		Address string `env:"ADDRESS"`
	} `envPrefix:"CERTIFICATE_MANAGER_"`

	// DiscoveryURL optionally overrides the Cloud.ru product discovery endpoint.
	DiscoveryURL string `env:"DISCOVERY_URL"`

	// ProjectID scopes both sources to the same Cloud.ru project.
	ProjectID string `env:"PROJECT_ID,required"`

	// RootFolder limits Secret Manager synchronization to a folder.
	RootFolder RootFolder `env:"ROOT_FOLDER"`
	// RootFolderOmitPrefix removes RootFolder from logical Secret Manager names.
	RootFolderOmitPrefix bool `env:"ROOT_FOLDER_OMIT_PREFIX"`
}

func (c Config) Validate() error {
	if c.RootFolderOmitPrefix && c.RootFolder == "" {
		return errors.New("CLOUDRU_ROOT_FOLDER must be set when CLOUDRU_ROOT_FOLDER_OMIT_PREFIX=true")
	}

	return nil
}

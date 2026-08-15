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
	IAM struct {
		Address string `env:"ADDRESS"`

		ClientID     string `env:"CLIENT_ID,file,required,notEmpty"`
		ClientSecret string `env:"CLIENT_SECRET,file,required,notEmpty" json:"-"`
	} `envPrefix:"IAM_"`
	CSM struct {
		Address string `env:"ADDRESS"`
	} `envPrefix:"CSM_"`

	DiscoveryURL string `env:"DISCOVERY_URL"`

	ProjectID string `env:"PROJECT_ID,required"`

	RootFolder           RootFolder `env:"ROOT_FOLDER"`
	RootFolderOmitPrefix bool       `env:"ROOT_FOLDER_OMIT_PREFIX"`
}

func (c Config) Validate() error {
	if c.RootFolderOmitPrefix && c.RootFolder == "" {
		return errors.New("CLOUDRU_ROOT_FOLDER must be set when CLOUDRU_ROOT_FOLDER_OMIT_PREFIX=true")
	}

	return nil
}

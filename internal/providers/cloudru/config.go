package cloudru

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
}

package api

type AuthConfig struct {
	// Token is a path to a file containing a Vault runtime token.
	Token string `env:"TOKEN,file" json:"-"`

	AppRole AppRoleConfig `envPrefix:"APPROLE_"`
}

type AppRoleConfig struct {
	// RoleID is Vault AppRole role_id bootstrap credential.
	RoleID string `env:"ROLE_ID,file" json:"-"`
	// SecretID is Vault AppRole secret_id bootstrap credential.
	SecretID string `env:"SECRET_ID,file" json:"-"`
}

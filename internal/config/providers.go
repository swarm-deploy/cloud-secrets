package config

type ProviderName string

const (
	ProviderNameCloudru ProviderName = "cloudru"
)

var (
	ProviderNames = []string{
		string(ProviderNameCloudru),
		"test",
	}
)

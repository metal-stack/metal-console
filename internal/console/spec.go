package console

import "time"

// Specification configures the application via environment.
type Specification struct {
	Port                   int    `default:"2222"`
	BmcReverseProxyAddress string `default:"" split_words:"true"`

	// metal-apiserver (v2) configuration items
	MetalAPIServerURL       string        `default:"http://localhost:8080" envconfig:"metal_apiserver_url"`
	TokenFile               string        `default:"" envconfig:"token_file"`
	TokenFileRereadDuration time.Duration `default:"1h" envconfig:"token_file_reread_duration"`

	// old metal-api based configuration items, can be removed once v2 migration is complete
	MetalAPIURL    string `default:"http://localhost:8080" envconfig:"metal_api_url"`
	HMACKey        string `default:"" envconfig:"hmac_key"`
	AdminGroupName string `default:"maas-all-all-admin" envconfig:"admin_group_name" split_words:"true"`
}

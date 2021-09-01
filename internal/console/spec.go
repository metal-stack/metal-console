package console

// Specification configures the application via environment.
type Specification struct {
	BindAddress            string `default:"localhost"`
	MetalAPIURL            string `default:"http://localhost:8080" envconfig:"metal_api_url"`
	Port                   int    `default:"2222"`
	HMACKey                string `default:"" envconfig:"hmac_key"`
	PublicKey              string `default:"" split_words:"true"`
	BmcReverseProxyAddress string `default:"" split_words:"true"`
	AdminGroupName         string `default:"" envconfig:"admin_group_name" split_words:"true"`
}

func (s *Specification) DevMode() bool {
	return len(s.PublicKey) > 0
}

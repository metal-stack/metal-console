package console

// Specification configures the application via environment.
type Specification struct {
	BindAddress             string `default:"localhost"`
	MetalAPIServerURL       string `default:"http://localhost:8080" envconfig:"metal_apiserver_url"`
	Port                    int    `default:"2222"`
	Token                   string `default:"" envconfig:"token"`
	TokenRenewalPersistence bool   `envconfig:"token_renew_persistence"`
	TokenRenewalNamespace   string `default:"" envconfig:"token_renewal_namespace"`
	TokenRenewalSecretName  string `default:"" envconfig:"token_renewal_secret_name"`
	TokenRenewalSecretKey   string `default:"token" envconfig:"token_renewal_secret_key"`
	PublicKey               string `default:"" split_words:"true"`
	BmcReverseProxyAddress  string `default:"" split_words:"true"`
}

func (s *Specification) DevMode() bool {
	return len(s.PublicKey) > 0
}

package server

// Specification configures the application via environment.
type Specification struct {
	BindAddress     string `default:"localhost"`
	MetalAPIAddress string `default:"localhost:8080" envconfig:"metal_api_address"`
	Port            int    `default:"2222"`
	PublicKey       string `default:"" split_words:"true"` // path to public SSH key (activates DevMode)
	HMACKey         string `envconfig:"hmac_key"`
}

func (s *Specification) DevMode() bool {
	return len(s.PublicKey) > 0
}

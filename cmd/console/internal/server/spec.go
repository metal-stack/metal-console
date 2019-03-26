package server

// Specification configures the application via environment.
type Specification struct {
	BindAddress string `default:"localhost"`
	APIAddress  string `default:"localhost:8080" envconfig:"metal_api_address"`
	MgmtAddress string `default:"localhost:3333" envconfig:"metal_mgmt_address"`
	Port        int    `default:"2222"`
	PublicKey   string `default:"" split_words:"true"` // path to public SSH key (activates DevMode)
}

func (s *Specification) DevMode() bool {
	return len(s.PublicKey) > 0
}

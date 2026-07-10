package console

import "time"

// Specification configures the application via environment.
type Specification struct {
	BindAddress             string        `default:"localhost"`
	MetalAPIServerURL       string        `default:"http://localhost:8080" envconfig:"metal_apiserver_url"`
	Port                    int           `default:"2222"`
	TokenFile               string        `default:"" envconfig:"token_file"`
	TokenFileRereadDuration time.Duration `default:"1h" envconfig:"token_file_reread_duration"`
	PublicKey               string        `default:"" split_words:"true"`
	BmcReverseProxyAddress  string        `default:"" split_words:"true"`
}

func (s *Specification) DevMode() bool {
	return len(s.PublicKey) > 0
}

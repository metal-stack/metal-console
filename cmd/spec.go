package cmd

// Specification configures the application via environment.
type Specification struct {
	MetalAPIUrl string `envconfig:"metal_api_url" default:"localhost:8080"`
	Port        int    `default:"2222"`
}

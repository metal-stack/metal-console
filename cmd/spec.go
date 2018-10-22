package cmd

// Specification configures the application via environment.
type Specification struct {
	MetalAPIUrl string `default:"localhost:8080"`
	Port        int    `default:"2222"`
}

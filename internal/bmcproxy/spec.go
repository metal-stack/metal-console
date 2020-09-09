package bmcproxy

// Specification configures the application via environment.
type Specification struct {
	Port int `default:"4444"`
}

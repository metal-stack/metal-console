package console

// Specification configures the application via environment.
type Specification struct {
	// Port where to listen for ssh connections
	Port int `default:"2222"`

	// SSH Keys
	PrivateKeyFile string `default:"/certs/server-key.pem" envconfig:"private_key_file"`
	PublicKeyFile  string `default:"/certs/server-key.pub" envconfig:"public_key_file"`

	// metal-bmc server mtls keys
	BMCCACertFile string `default:"/certs/ca.pem" envconfig:"bmc_ca_cert_file"`
	BMCCertFile   string `default:"/certs/client.pem" envconfig:"bmc_cert_file"`
	BMCKeyFile    string `default:"/certs/client-key.pem" envconfig:"bmc_key_file"`

	// metal-apiserver (v2) configuration items
	MetalAPIServerURL string `default:"http://localhost:8080" envconfig:"metal_apiserver_url"`
	TokenFile         string `default:"" envconfig:"token_file"`

	// old metal-api based configuration items, can be removed once v2 migration is complete
	MetalAPIURL    string `default:"http://localhost:8080" envconfig:"metal_api_url"`
	AdminGroupName string `default:"maas-all-all-admin" envconfig:"admin_group_name" split_words:"true"`
}

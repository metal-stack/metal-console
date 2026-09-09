package console

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"
)

// Specification configures the application via environment.
type Specification struct {
	// Port where to listen for ssh connections
	Port int `default:"2222"`

	// SSH Keys
	PrivateKeyFile string `default:"/certs/server-key.pem" envconfig:"private_key_file"`
	PublicKeyFile  string `default:"/certs/server-key.pub" envconfig:"public_key_file"`

	// Parsed SSH Keys
	publicKey  ssh.PublicKey
	privateKey gossh.Signer

	// metal-bmc server mtls keys
	BMCCACertFile string `default:"/certs/ca.pem" envconfig:"bmc_ca_cert_file"`
	BMCCertFile   string `default:"/certs/client.pem" envconfig:"bmc_cert_file"`
	BMCKeyFile    string `default:"/certs/client-key.pem" envconfig:"bmc_key_file"`

	// Parsed server keys
	clientCert tls.Certificate
	caCertPool *x509.CertPool

	// metal-apiserver (v2) configuration items
	MetalAPIServerURL string `default:"http://localhost:8080" envconfig:"metal_apiserver_url"`
	TokenFile         string `default:"" envconfig:"token_file"`

	// old metal-api based configuration items, can be removed once v2 migration is complete
	MetalAPIURL    string `default:"http://localhost:8080" envconfig:"metal_api_url"`
	AdminGroupName string `default:"maas-all-all-admin" envconfig:"admin_group_name" split_words:"true"`
}

func (s *Specification) Parse() error {
	// Parse SSH Keys
	bb, err := os.ReadFile(s.PublicKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load public host key:%w", err)
	}
	pubHostKey, _, _, _, err := ssh.ParseAuthorizedKey(bb)
	if err != nil {
		return fmt.Errorf("failed to parse public host key: %w", err)
	}
	s.publicKey = pubHostKey

	serverKey, err := os.ReadFile(s.PrivateKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load private host key:%w", err)
	}

	hostKey, err := gossh.ParsePrivateKey(serverKey)
	if err != nil {
		return fmt.Errorf("failed to load host key %w", err)
	}
	s.privateKey = hostKey

	// Parse BMC Certs

	clientCert, err := tls.LoadX509KeyPair(s.BMCCertFile, s.BMCKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(s.BMCCACertFile)
	if err != nil {
		return fmt.Errorf("failed to load CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if !ok {
		return errors.New("bad ca certificate")
	}
	s.clientCert = clientCert
	s.caCertPool = caCertPool

	return nil
}

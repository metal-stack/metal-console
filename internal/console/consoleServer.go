package console

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	metalgo "github.com/metal-stack/metal-go"
	"github.com/metal-stack/metal-go/api/models"

	"github.com/gliderlabs/ssh"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
)

type consoleServer struct {
	log    *zap.SugaredLogger
	client *metalgo.Driver
	spec   *Specification
	ips    *sync.Map
}

func NewServer(log *zap.SugaredLogger, spec *Specification) (*consoleServer, error) {
	client, err := newClient(spec.MetalAPIURL, spec.HMACKey)
	if err != nil {
		return nil, err
	}
	return &consoleServer{
		log:    log,
		client: client,
		spec:   spec,
		ips:    new(sync.Map),
	}, nil
}

// Run starts ssh server and listen for console connections.
func (cs *consoleServer) Run() {
	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", cs.spec.Port),
		Handler:          cs.sessionHandler,
		PublicKeyHandler: cs.authHandler,
	}

	hostKey, err := loadHostKey()
	if err != nil {
		cs.log.Errorw("failed to load host key", "error", err)
		runtime.Goexit()
	}
	s.AddHostKey(hostKey)

	cs.log.Infow("starting ssh server", "port", cs.spec.Port)
	cs.log.Fatal(s.ListenAndServe())
}

const oidcEnv = "LC_METAL_STACK_OIDC_TOKEN"

func (cs *consoleServer) sessionHandler(s ssh.Session) {
	machineID := s.User()
	defer cs.ips.Delete(machineID)

	m, err := cs.getMachine(machineID)
	if err != nil {
		cs.log.Errorw("failed to fetch requested machine", "machineID", machineID, "error", err)
		err = s.Exit(1)
		if err != nil {
			cs.log.Errorw("failed to exit SSH session", "error", err)
		}
	}

	// If the machine is a firewall
	// check if the ssh session contains the oidc token and the user is member of providertenant
	// ssh client can pass environment variables, but only environment variables starting with LC_ are passed
	// OIDC token must be stored in LC_METAL_STACK_OIDC_TOKEN
	if m.Allocation != nil && m.Allocation.Role != nil && *m.Allocation.Role == models.V1MachineAllocationRoleFirewall {
		environ := s.Environ()
		token := ""
		for _, env := range environ {
			if strings.HasPrefix(env, oidcEnv+"=") {
				parts := strings.Split(env, "=")
				if len(parts) == 2 {
					token = parts[1]
				}
			}
		}
		if token == "" {
			cs.log.Errorw("unable to find OIDC token stored in %s env variable which is required for firewall console access", oidcEnv)
			err = s.Exit(1)
			if err != nil {
				cs.log.Errorw("failed to exit SSH session", "error", err)
			}
		}
		user, err := cs.client.UserGet(token)
		if err != nil {
			cs.log.Errorw("failed to fetch user details from oidc token", "machineID", machineID, "error", err)
			err = s.Exit(1)
			if err != nil {
				cs.log.Errorw("failed to exit SSH session", "error", err)
			}
		}
		isAdmin := false
		for _, g := range user.User.Groups {
			if g == cs.spec.AdminGroupName {
				isAdmin = true
			}
		}
		if !isAdmin {
			cs.log.Errorw("you are not member of required admin group to access this machine console", "machineID", machineID, "group", cs.spec.AdminGroupName)
			err = s.Exit(1)
			if err != nil {
				cs.log.Errorw("failed to exit SSH session", "error", err)
			}
		}
	}

	defer func() {
		err = s.Exit(1)
		if err != nil {
			cs.log.Errorw("failed to exit SSH session", "error", err)
		}
	}()

	mgmtServiceAddress := m.Partition.Mgmtserviceaddress

	if cs.spec.DevMode() {
		mgmtServiceAddress = cs.spec.BmcReverseProxyAddress
	}

	tcpConn, err := cs.connectToManagementNetwork(mgmtServiceAddress)
	if err != nil {
		cs.log.Errorw("failed to connect to management network", "error", err)
		return
	}
	defer tcpConn.Close()

	sshConn, sshClient, sshSession, err := cs.connectSSH(tcpConn, mgmtServiceAddress, machineID)
	if err != nil {
		cs.log.Errorw("failed to establish SSH connection via already established TCP connection", "error", err)
		return
	}
	defer func() {
		sshSession.Close()
		sshClient.Close()
		sshConn.Close()
	}()

	cs.sendIPMIData(sshSession, machineID)

	cs.requestPTY(sshSession)

	done := make(chan bool)

	cs.redirectIO(s, sshSession, done)

	err = sshSession.Start("bash")
	if err != nil {
		cs.log.Errorw("failed to start bash via SSH session", "error", err)
		return
	}

	// wait till connection is closed
	<-done
}

func (cs *consoleServer) redirectIO(callerSSHSession ssh.Session, machineSSHSession *gossh.Session, done chan<- bool) {
	stdin, err := machineSSHSession.StdinPipe()
	if err != nil {
		cs.log.Errorw("failed to fetch stdin for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(stdin, callerSSHSession)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Errorw("failed to copy caller stdin to machine", "error", err)
			}

			done <- true
		}()
	}

	stdout, err := machineSSHSession.StdoutPipe()
	if err != nil {
		cs.log.Errorw("failed to fetch stdout for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stdout)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Errorw("failed to copy machine stdout to caller", "error", err)
			}

			done <- true
		}()
	}

	stderr, err := machineSSHSession.StderrPipe()
	if err != nil {
		cs.log.Errorw("failed to fetch stderr for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stderr)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Errorw("failed to copy machine stderr to caller", "error", err)
			}

			done <- true
		}()
	}
}

func (cs *consoleServer) requestPTY(sshSession *gossh.Session) {
	modes := gossh.TerminalModes{
		gossh.ECHO:          0,      // disable echoing
		gossh.TTY_OP_ISPEED: 115200, // input speed in baud
		gossh.TTY_OP_OSPEED: 115200, // output speed in baud
	}

	if err := sshSession.RequestPty("xterm", 80, 40, modes); err != nil {
		cs.log.Errorw("failed to request PTY", "error", err)
	}
}

func (cs *consoleServer) connectSSH(tcpConn *tls.Conn, mgmtServiceAddress, machineID string) (gossh.Conn, *gossh.Client, *gossh.Session, error) {
	pubHostKey, err := loadPublicHostKey()
	if err != nil {
		cs.log.Errorw("failed to load public host key", "error", err)
		return nil, nil, nil, err
	}

	sshConfig := &gossh.ClientConfig{
		User:            machineID,
		HostKeyCallback: gossh.FixedHostKey(pubHostKey),
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(tcpConn, mgmtServiceAddress, sshConfig)
	if err != nil {
		cs.log.Errorw("failed to open client connection", "mgmt-service address", mgmtServiceAddress, "error", err)
		return nil, nil, nil, err
	}

	sshClient := gossh.NewClient(sshConn, chans, reqs)

	sshSession, err := sshClient.NewSession()
	if err != nil {
		cs.log.Errorw("failed to create new SSH session", "error", err)
		return nil, nil, nil, err
	}

	return sshConn, sshClient, sshSession, nil
}

func (cs *consoleServer) connectToManagementNetwork(mgmtServiceAddress string) (*tls.Conn, error) {
	clientCert, err := tls.LoadX509KeyPair("/certs/client.pem", "/certs/client-key.pem")
	if err != nil {
		cs.log.Errorw("failed to load client certificate", "cert", "/certs/client.pem", "key", "/certs/client-key.pem", "error", err)
		return nil, err
	}

	caCert, err := os.ReadFile("/certs/ca.pem")
	if err != nil {
		cs.log.Errorw("failed to load CA certificate", "cert", "/certs/ca.pem", "error", err)
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if !ok {
		cs.log.Errorw("failed to append CA certificate")
		return nil, errors.New("bad ca certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	tcpConn, err := tls.Dial("tcp", mgmtServiceAddress, tlsConfig)
	if err != nil {
		cs.log.Errorw("failed to dial via TCP", "address", mgmtServiceAddress, "error", err)
		return nil, err
	}
	cs.log.Infow("connect to management network", "remote addr", tcpConn.RemoteAddr())

	return tcpConn, nil
}

func (cs *consoleServer) sendIPMIData(sshSession *gossh.Session, machineID string) {
	m, err := cs.getMachineIPMI(machineID)
	if err != nil {
		cs.log.Errorw("failed to fetch IPMI data from Metal API", "machineID", machineID, "error", err)
		runtime.Goexit()
	}

	ipmiData, err := m.Ipmi.MarshalBinary()
	if err != nil {
		cs.log.Errorw("failed to marshal MetalIPMI", "error", err)
		runtime.Goexit()
	}

	err = sshSession.Setenv("LC_IPMI_DATA", string(ipmiData))
	if err != nil {
		cs.log.Errorw("failed to send IPMI data to BMC proxy", "error", err)
		runtime.Goexit()
	}
}

func (cs *consoleServer) authHandler(ctx ssh.Context, publicKey ssh.PublicKey) bool {
	machineID := ctx.User()
	cs.log.Infow("authHandler", "publicKey", publicKey)
	knownAuthorizedKeys, err := cs.getAuthorizedKeysForMachine(machineID)
	if err != nil {
		cs.log.Errorw("no authorized keys found", "machineID", machineID, "error", err)
		return false
	}
	for _, key := range knownAuthorizedKeys {
		cs.log.Infow("authHandler", "machineID", machineID, "authorizedKey", key)
		same := ssh.KeysEqual(publicKey, key)
		if same {
			return true
		}
	}
	cs.log.Warnw("no matching authorized key found", "machineID", machineID)
	return false
}

func (cs *consoleServer) getAuthorizedKeysForMachine(machineID string) ([]ssh.PublicKey, error) {
	resp, err := cs.getMachine(machineID)
	if err != nil {
		cs.log.Errorw("failed to fetch requested machine", "machineID", machineID, "error", err)
		return nil, err
	}
	if resp == nil {
		cs.log.Errorw("requested machine is nil", "machineID", machineID)
		return nil, err
	}

	if cs.spec.DevMode() {
		bb, err := os.ReadFile(cs.spec.PublicKey)
		if err != nil {
			cs.log.Errorw("failed to read public key", "file", cs.spec.PublicKey)
			return nil, err
		}
		resp.Allocation.SSHPubKeys = []string{
			string(bb),
		}
	}

	privateIP := ""
	if resp.Allocation != nil {
		for _, nw := range resp.Allocation.Networks {
			if *nw.Private {
				if len(nw.Ips) > 0 {
					privateIP = nw.Ips[0]
					break
				}
			}
		}
	}
	if privateIP == "" {
		return nil, fmt.Errorf("failed to detect private IP of machine:%s", machineID)
	}
	cs.ips.Store(machineID, privateIP)

	var pubKeys []ssh.PublicKey
	for _, key := range resp.Allocation.SSHPubKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("error parsing public key:%w", err)
		}
		pubKeys = append(pubKeys, pubKey)
	}

	return pubKeys, nil
}

func loadHostKey() (gossh.Signer, error) {
	bb, err := os.ReadFile("/certs/server-key.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to load private host key:%w", err)
	}
	return gossh.ParsePrivateKey(bb)
}

func loadPublicHostKey() (gossh.PublicKey, error) {
	bb, err := os.ReadFile("/certs/server-key.pub")
	if err != nil {
		return nil, fmt.Errorf("failed to load public host key:%w", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(bb)
	return pubKey, err
}

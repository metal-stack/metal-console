package console

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	metalgo "github.com/metal-stack/metal-go"
	"github.com/metal-stack/metal-go/api/client/machine"
	"github.com/metal-stack/metal-go/api/client/user"
	"github.com/metal-stack/metal-go/api/models"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type consoleServer struct {
	log        *slog.Logger
	client     metalgo.Client
	spec       *Specification
	createdAts *sync.Map
}

func NewServer(log *slog.Logger, spec *Specification, client metalgo.Client) *consoleServer {
	return &consoleServer{
		log:        log,
		client:     client,
		spec:       spec,
		createdAts: new(sync.Map),
	}
}

// Run starts ssh server and listen for console connections.
func (cs *consoleServer) Run() error {
	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", cs.spec.Port),
		Handler:          cs.sessionHandler,
		PublicKeyHandler: cs.publicKeyHandler,
		PasswordHandler:  cs.passwordHandler,
	}

	hostKey, err := loadHostKey()
	if err != nil {
		return fmt.Errorf("failed to load host key %w", err)
	}
	s.AddHostKey(hostKey)

	cs.log.Info("starting ssh server", "port", cs.spec.Port)
	err = s.ListenAndServe()
	if err != nil {
		return fmt.Errorf("unable to start listener %w", err)
	}
	return nil
}

const oidcEnv = "LC_METAL_STACK_OIDC_TOKEN"

func (cs *consoleServer) sessionHandler(s ssh.Session) {
	machineID := s.User()

	resp, err := cs.client.Machine().FindMachine(machine.NewFindMachineParams().WithID(machineID), nil)
	if err != nil || resp == nil || resp.Payload == nil {
		cs.log.Error("failed to fetch requested machine", "machineID", machineID, "error", err)
		cs.exitSession(s)
		return
	}

	m := resp.Payload
	var isAdmin bool

	// If the machine is a firewall or not allocated
	// check if the ssh session contains the oidc token and the user is member of admin group
	// ssh client can pass environment variables, but only environment variables starting with LC_ are passed
	// OIDC token must be stored in LC_METAL_STACK_OIDC_TOKEN
	if (m.Allocation == nil) || (m.Allocation != nil && m.Allocation.Role != nil && *m.Allocation.Role == models.V1MachineAllocationRoleFirewall) {
		token := ""
		for _, env := range s.Environ() {
			_, t, found := strings.Cut(env, oidcEnv+"=")
			if found {
				token = t
				break
			}
		}

		isAdmin, err = cs.checkIsAdmin(machineID, token)
		if err != nil {
			_, _ = io.WriteString(s, err.Error()+"\n")
			cs.exitSession(s)
			return
		}
	}

	mgmtServiceAddress := m.Partition.Mgmtserviceaddress

	if cs.spec.DevMode() {
		mgmtServiceAddress = cs.spec.BmcReverseProxyAddress
	}

	tcpConn, err := cs.connectToManagementNetwork(mgmtServiceAddress)
	if err != nil {
		cs.log.Error("failed to connect to management network", "error", err)
		return
	}
	defer tcpConn.Close()

	sshConn, sshClient, sshSession, err := cs.connectSSH(tcpConn, mgmtServiceAddress, machineID)
	if err != nil {
		cs.log.Error("failed to establish SSH connection via already established TCP connection", "error", err)
		return
	}
	defer func() {
		sshSession.Close()
		sshClient.Close()
		sshConn.Close()
	}()

	cs.requestPTY(sshSession)

	done := make(chan bool)

	cs.redirectIO(s, sshSession, done)

	// check periodically if the session is still allowed.
	if !isAdmin {
		go cs.terminateIfPublicKeysChanged(s)
	}

	err = sshSession.Start("bash")
	if err != nil {
		cs.log.Error("failed to start bash via SSH session", "error", err)
		return
	}

	// wait till connection is closed
	<-done
}

func (cs *consoleServer) terminateIfPublicKeysChanged(s ssh.Session) {
	machineID := s.User()
	createdAt, ok := cs.createdAts.Load(machineID)
	if !ok {
		_, _ = io.WriteString(s, "machine allocation not known, terminating console session\n")
		cs.log.Info("machine allocation not known, terminating ssh session", "machineID", machineID)
		cs.exitSession(s)
		return
	}

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.Context().Done():
			cs.log.Info("connection closed", "machineID", machineID)
			return
		case <-ticker.C:
			cs.log.Info("checking if machine is still owned by the same user", "machineID", machineID)

			m, err := cs.client.Machine().FindMachine(machine.NewFindMachineParams().WithID(machineID), nil)
			if err != nil {
				cs.log.Error("unable to load machine", "machineID", machineID, "error", err)
				continue
			}
			if m.Payload.Allocation == nil {
				_, _ = io.WriteString(s, "machine is not allocated anymore, terminating console session\n")
				cs.log.Info("machine is not allocated anymore, terminating ssh session", "machineID", machineID)
				cs.exitSession(s)
				return
			}
			if createdAt != m.Payload.Allocation.Created.String() {
				_, _ = io.WriteString(s, "machine allocation changed, terminating console session\n")
				cs.log.Info("machine allocation changed, terminating ssh session", "machineID", machineID, "old-ts", createdAt, "new-ts", m.Payload.Allocation.Created.String())
				cs.exitSession(s)
				return
			}
		}
	}
}

func (cs *consoleServer) exitSession(session ssh.Session) {
	err := session.Exit(1)
	if err != nil {
		cs.log.Error("failed to exit SSH session", "error", err)
	}
}

func (cs *consoleServer) redirectIO(callerSSHSession ssh.Session, machineSSHSession *gossh.Session, done chan<- bool) {
	stdin, err := machineSSHSession.StdinPipe()
	if err != nil {
		cs.log.Error("failed to fetch stdin for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(stdin, callerSSHSession)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Error("failed to copy caller stdin to machine", "error", err)
			}

			done <- true
		}()
	}

	stdout, err := machineSSHSession.StdoutPipe()
	if err != nil {
		cs.log.Error("failed to fetch stdout for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stdout)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Error("failed to copy machine stdout to caller", "error", err)
			}

			done <- true
		}()
	}

	stderr, err := machineSSHSession.StderrPipe()
	if err != nil {
		cs.log.Error("failed to fetch stderr for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stderr)
			if err != nil && !errors.Is(err, io.EOF) {
				cs.log.Error("failed to copy machine stderr to caller", "error", err)
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
		cs.log.Error("failed to request PTY", "error", err)
	}
}

func (cs *consoleServer) connectSSH(tcpConn *tls.Conn, mgmtServiceAddress, machineID string) (gossh.Conn, *gossh.Client, *gossh.Session, error) {
	pubHostKey, err := loadPublicHostKey()
	if err != nil {
		cs.log.Error("failed to load public host key", "error", err)
		return nil, nil, nil, err
	}

	sshConfig := &gossh.ClientConfig{
		User:            machineID,
		HostKeyCallback: gossh.FixedHostKey(pubHostKey),
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(tcpConn, mgmtServiceAddress, sshConfig)
	if err != nil {
		cs.log.Error("failed to open client connection", "mgmt-service address", mgmtServiceAddress, "error", err)
		return nil, nil, nil, err
	}

	sshClient := gossh.NewClient(sshConn, chans, reqs)

	sshSession, err := sshClient.NewSession()
	if err != nil {
		cs.log.Error("failed to create new SSH session", "error", err)
		return nil, nil, nil, err
	}

	return sshConn, sshClient, sshSession, nil
}

func (cs *consoleServer) connectToManagementNetwork(mgmtServiceAddress string) (*tls.Conn, error) {
	clientCert, err := tls.LoadX509KeyPair("/certs/client.pem", "/certs/client-key.pem")
	if err != nil {
		cs.log.Error("failed to load client certificate", "cert", "/certs/client.pem", "key", "/certs/client-key.pem", "error", err)
		return nil, err
	}

	caCert, err := os.ReadFile("/certs/ca.pem")
	if err != nil {
		cs.log.Error("failed to load CA certificate", "cert", "/certs/ca.pem", "error", err)
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if !ok {
		cs.log.Error("failed to append CA certificate")
		return nil, errors.New("bad ca certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	tcpConn, err := tls.Dial("tcp", mgmtServiceAddress, tlsConfig)
	if err != nil {
		cs.log.Error("failed to dial via TCP", "address", mgmtServiceAddress, "error", err)
		return nil, err
	}
	cs.log.Info("connect to management network", "remote addr", tcpConn.RemoteAddr())

	return tcpConn, nil
}

func (cs *consoleServer) publicKeyHandler(ctx ssh.Context, publicKey ssh.PublicKey) bool {
	machineID := ctx.User()

	cs.log.Info("authHandler", "publicKey", publicKey)
	knownAuthorizedKeys, err := cs.getAuthorizedKeysForMachine(machineID)
	if err != nil {
		cs.log.Error("abort establishment of console session", "machineID", machineID, "error", err)
		return false
	}
	for _, key := range knownAuthorizedKeys {
		cs.log.Info("authHandler", "machineID", machineID, "authorizedKey", key)
		same := ssh.KeysEqual(publicKey, key)
		if same {
			return true
		}
	}

	cs.log.Warn("no matching authorized key found", "machineID", machineID)

	return false
}

func (cs *consoleServer) getAuthorizedKeysForMachine(machineID string) ([]ssh.PublicKey, error) {
	resp, err := cs.client.Machine().FindMachine(machine.NewFindMachineParams().WithID(machineID), nil)
	if err != nil {
		cs.log.Error("failed to fetch requested machine", "machineID", machineID, "error", err)
		return nil, err
	}
	if resp.Payload == nil || resp.Payload.Allocation == nil {
		cs.log.Error("requested machine is nil", "machineID", machineID)
		return nil, fmt.Errorf("no machine found with id: %s", machineID)
	}
	alloc := resp.Payload.Allocation

	cs.createdAts.Store(machineID, alloc.Created.String())

	if cs.spec.DevMode() {
		bb, err := os.ReadFile(cs.spec.PublicKey)
		if err != nil {
			cs.log.Error("failed to read public key", "file", cs.spec.PublicKey)
			return nil, err
		}
		alloc.SSHPubKeys = []string{
			string(bb),
		}
	}

	var pubKeys []ssh.PublicKey
	for _, key := range alloc.SSHPubKeys {
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

func (cs *consoleServer) passwordHandler(ctx ssh.Context, password string) bool {
	isAdmin, err := cs.checkIsAdmin(ctx.User(), password)
	if err != nil {
		cs.log.Error("error evaluating if user is admin", "error", err)
		return false
	}

	return isAdmin
}

func (cs *consoleServer) checkIsAdmin(machineID string, token string) (bool, error) {
	if token == "" {
		return false, fmt.Errorf("unable to find OIDC token stored in %s env variable which is required for machine console access", oidcEnv)
	}

	metal, err := metalgo.NewDriver(cs.spec.MetalAPIURL, token, "")
	if err != nil {
		return false, fmt.Errorf("failed to create metal client: %w", err)
	}

	user, err := metal.User().GetMe(user.NewGetMeParams(), nil)
	if err != nil {
		cs.log.Error("failed to fetch user details from oidc token", "machineID", machineID, "error", err, "token", token)
		return false, fmt.Errorf("given oidc token is invalid")
	}

	isAdmin := false
	for _, g := range user.Payload.Groups {
		if g == cs.spec.AdminGroupName {
			isAdmin = true
		}
	}
	if !isAdmin {
		return false, fmt.Errorf("you are not member of required admin group:%s to access this machine console", cs.spec.AdminGroupName)
	}

	return true, nil
}

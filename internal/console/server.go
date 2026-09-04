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

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ssh "github.com/tailscale/gliderssh"

	gossh "golang.org/x/crypto/ssh"
)

type consoleServer struct {
	log        *slog.Logger
	spec       *Specification
	createdAts *sync.Map

	// newMetal creates the metal adapter for a session. It is a field to allow
	// test injection of a mock adapter.
	newMetal func(log *slog.Logger, token, project string, spec Specification) (metal, error)
	// connectMachine establishes the connection to the machine's serial console and
	// returns a cleanup func and the machine-side ssh session. It is a field to allow
	// test injection.
	connectMachine func(mgmtServiceAddress, machineID string) (func(), *gossh.Session, error)
}

func NewServer(log *slog.Logger, spec *Specification) *consoleServer {
	cs := &consoleServer{
		log:        log,
		spec:       spec,
		createdAts: new(sync.Map),
	}
	cs.newMetal = newMetal
	cs.connectMachine = cs.realConnectMachine
	return cs
}

// Run starts ssh server and listen for console connections.
func (cs *consoleServer) Run() error {
	s := &ssh.Server{
		Addr:    fmt.Sprintf(":%d", cs.spec.Port),
		Handler: cs.sessionHandler,
		// does not have access to the token, must be called to be able to store the publicKey in the session
		// which will be then checked against the stored publickey in the machine by sessionHandler
		PublicKeyHandler: cs.noopPublicKeyHandler,
		Banner:           "metal-stack.io console server\n",
	}

	serverKey, err := os.ReadFile(cs.spec.PrivateKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load private host key:%w", err)
	}

	hostKey, err := gossh.ParsePrivateKey(serverKey)
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

func (cs *consoleServer) sessionHandler(s ssh.Session) {
	var (
		machineID      = s.User()
		token, project = tokenAndProjectFromSessionEnv(s)
	)

	metal, err := cs.newMetal(cs.log, token, project, *cs.spec)
	if err != nil {
		cs.log.Error("error constructing metal adapter", "error", err)
		cs.exitSession(s, err)
		return
	}

	isAdmin, err := metal.checkIsAuthenticated(s.Context())
	if err != nil {
		cs.log.Error("check for authentication failed", "error", err)
		cs.exitSession(s, err)
		return
	}

	machine, err := metal.getMachine(s.Context(), machineID)
	if err != nil {
		cs.log.Error("failed to fetch machine", "error", err)
		cs.exitSession(s, err)
		return
	}

	if !isAdmin {
		if err := cs.checkAuthorizedKeys(*machine, s.PublicKey()); err != nil {
			cs.log.Error("public key does not match", "error", err)
			cs.exitSession(s, err)
			return
		}
	}

	cs.createdAts.Store(machineID, machine.createdAt.String())
	defer cs.createdAts.Delete(machineID)

	if !isAdmin && machine.role == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		// If the machine is a not a regular machine, i.e. a firewall, or an admin wants access to an arbitrary machine
		// check if the ssh session contains the oidc token and the user is member of admin group
		// ssh client can pass environment variables, but only environment variables starting with LC_ are passed
		// OIDC token must be stored in LC_METAL_STACK_OIDC_TOKEN
		cs.log.Error("prevented non admin access to a firewall console", "machineID", machineID, "role", machine.role, "from", s.RemoteAddr())
		cs.exitSession(s, fmt.Errorf("only admins can access firewall console"))
		return
	}

	mgmtServiceAddresses := machine.managementServerAddresses
	if len(mgmtServiceAddresses) == 0 {
		cs.log.Error("failed to connect to management network, no management server address given")
		cs.exitSession(s, err)
		return
	}

	// TODO try all available addresses round robin
	mgmtServiceAddress := mgmtServiceAddresses[0]

	cleanup, sshSession, err := cs.connectMachine(mgmtServiceAddress, machineID)
	if err != nil {
		cs.log.Error("failed to connect to machine console", "error", err)
		return
	}
	defer cleanup()

	cs.requestPTY(sshSession)

	wait := cs.redirectIO(s, sshSession)

	if !isAdmin {
		// check periodically if the session is still allowed.
		// admins don't need to be disconnected from machines
		go cs.terminateIfPublicKeysChanged(s, metal)
	}

	if err := sshSession.Start("bash"); err != nil {
		cs.log.Error("failed to start bash via SSH session", "error", err)
		return
	}

	// wait till all io is done
	wait()
}

func (cs *consoleServer) terminateIfPublicKeysChanged(s ssh.Session, metal metal) {
	machineID := s.User()
	createdAt, ok := cs.createdAts.Load(machineID)
	if !ok {
		_, _ = io.WriteString(s, "machine allocation not known, terminating console session\n")
		cs.log.Info("machine allocation not known, terminating ssh session", "machineID", machineID)
		cs.exitSession(s, fmt.Errorf("machine allocation not known, terminating ssh session"))
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
			// we must use adminv2 because otherwise project must be passed which is not known here
			m, err := metal.getMachine(s.Context(), machineID)
			if err != nil {
				cs.log.Error("unable to load machine", "machineID", machineID, "error", err)
				continue
			}
			if !m.allocated {
				_, _ = io.WriteString(s, "machine is not allocated anymore, terminating console session\n")
				cs.log.Info("machine is not allocated anymore, terminating ssh session", "machineID", machineID)
				cs.exitSession(s, fmt.Errorf("machine is not allocated anymore, terminating ssh session"))
				return
			}
			if createdAt != m.createdAt.String() {
				_, _ = io.WriteString(s, "machine allocation changed, terminating console session\n")
				cs.log.Info("machine allocation changed, terminating ssh session", "machineID", machineID, "old-ts", createdAt, "new-ts", m.createdAt.String())
				cs.exitSession(s, fmt.Errorf("machine allocation changed, terminating ssh session"))
				return
			}
		}
	}
}

func (cs *consoleServer) exitSession(session ssh.Session, err error) {
	_, _ = io.WriteString(session, err.Error()+"\n")
	if err := session.Exit(1); err != nil {
		cs.log.Error("failed to exit SSH session", "error", err)
	}
}

func (cs *consoleServer) redirectIO(callerSSHSession ssh.Session, machineSSHSession *gossh.Session) func() {
	var wg sync.WaitGroup

	stdin, err := machineSSHSession.StdinPipe()
	if err != nil {
		cs.log.Error("failed to fetch stdin for SSH session", "error", err)
	} else {
		wg.Go(func() {
			_, copyErr := io.Copy(stdin, callerSSHSession)
			if copyErr != nil && !errors.Is(copyErr, io.EOF) {
				cs.log.Error("failed to copy caller stdin to machine", "error", copyErr)
			}
		})
	}

	stdout, err := machineSSHSession.StdoutPipe()
	if err != nil {
		cs.log.Error("failed to fetch stdout for SSH session", "error", err)
	} else {
		wg.Go(func() {
			_, copyErr := io.Copy(callerSSHSession, stdout)
			if copyErr != nil && !errors.Is(copyErr, io.EOF) {
				cs.log.Error("failed to copy machine stdout to caller", "error", copyErr)
			}
		})
	}

	stderr, err := machineSSHSession.StderrPipe()
	if err != nil {
		cs.log.Error("failed to fetch stderr for SSH session", "error", err)
	} else {
		wg.Go(func() {
			_, copyErr := io.Copy(callerSSHSession, stderr)
			if copyErr != nil && !errors.Is(copyErr, io.EOF) {
				cs.log.Error("failed to copy machine stderr to caller", "error", copyErr)
			}
		})
	}

	return wg.Wait
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

func (cs *consoleServer) realConnectMachine(mgmtServiceAddress, machineID string) (func(), *gossh.Session, error) {
	tcpConn, err := cs.connectToManagementNetwork(mgmtServiceAddress)
	if err != nil {
		return nil, nil, err
	}

	sshConn, sshClient, sshSession, err := cs.connectSSH(tcpConn, mgmtServiceAddress, machineID)
	if err != nil {
		_ = tcpConn.Close()
		return nil, nil, err
	}

	cleanup := func() {
		_ = tcpConn.Close()
		_ = sshSession.Close()
		_ = sshClient.Close()
		_ = sshConn.Close()
	}
	return cleanup, sshSession, nil
}

func (cs *consoleServer) connectSSH(tcpConn *tls.Conn, mgmtServiceAddress, machineID string) (gossh.Conn, *gossh.Client, *gossh.Session, error) {
	bb, err := os.ReadFile(cs.spec.PublicKeyFile)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load public host key:%w", err)
	}
	pubHostKey, _, _, _, err := ssh.ParseAuthorizedKey(bb)
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
	clientCert, err := tls.LoadX509KeyPair(cs.spec.BMCCertFile, cs.spec.BMCKeyFile)
	if err != nil {
		cs.log.Error("failed to load client certificate", "cert", cs.spec.BMCCertFile, "key", cs.spec.BMCKeyFile, "error", err)
		return nil, err
	}

	caCert, err := os.ReadFile(cs.spec.BMCCACertFile)
	if err != nil {
		cs.log.Error("failed to load CA certificate", "cert", cs.spec.BMCCACertFile, "error", err)
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

func (cs *consoleServer) checkAuthorizedKeys(machine machine, publicKey ssh.PublicKey) error {
	if publicKey == nil {
		return fmt.Errorf("publicKey is nil")
	}
	for _, key := range machine.sshPublicKeys {
		cs.log.Debug("check if public key matches", "machine key", key, "authorized key", publicKey.Type())
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return fmt.Errorf("error parsing public key:%w", err)
		}
		same := ssh.KeysEqual(publicKey, key)
		if same {
			cs.log.Info("found matching public key for machine access", "machineID", machine.id)
			return nil
		}
	}
	return fmt.Errorf("no matching authorized key found for machineID:%s", machine.id)
}

func (cs *consoleServer) noopPublicKeyHandler(ctx ssh.Context, publicKey ssh.PublicKey) error {
	// This publicKeyHandler is only called to ensure the publicKey is stored in the ssh.Session.
	// without a publicKeyHandler it is not stored in the session
	machineID := ctx.User()
	cs.log.Info("evaluating machine console access with public key access", "machineID", machineID)
	return nil
}

func tokenAndProjectFromSessionEnv(s ssh.Session) (string, string) {
	var (
		token   string
		project string
	)
	for _, env := range s.Environ() {
		_, t, tfound := strings.Cut(env, oidcTokenEnv+"=")
		if tfound {
			token = t
		}
		_, p, pfound := strings.Cut(env, projectEnv+"=")
		if pfound {
			project = p
		}
	}

	return token, project
}

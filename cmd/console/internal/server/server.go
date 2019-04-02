package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/client/machine"
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/models"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"os"
	"sync"
)

type consoleServer struct {
	log           *zap.Logger
	machineClient *machine.Client
	spec          *Specification
	mutex         sync.RWMutex
	consoles      *sync.Map
}

func New(log *zap.Logger, spec *Specification) *consoleServer {
	return &consoleServer{
		log:           log,
		machineClient: newMachineClient(spec.MetalAPIAddress),
		spec:          spec,
		consoles:      &sync.Map{},
		mutex:         sync.RWMutex{},
	}
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
		cs.log.Sugar().Fatal("cannot load host key", "error", err)
		os.Exit(-1)
	}
	s.AddHostKey(hostKey)

	cs.log.Sugar().Info("starting ssh server", "port", cs.spec.Port)
	cs.log.Sugar().Fatal(s.ListenAndServe())
}

func (cs *consoleServer) sessionHandler(s ssh.Session) {
	machineID := s.User()

	c, err := cs.getConsole(machineID)
	if err != nil {
		cs.log.Sugar().Fatal("failed to get console", "machineID", machineID, "error", err)
		s.Exit(1)
		return
	}
	defer cs.consoles.Delete(machineID)

	tcpConn := cs.connectToManagementNetwork()
	defer tcpConn.Close()

	sshConn, sshClient, sshSession := cs.connectSSH(tcpConn, machineID)
	defer func() {
		sshSession.Close()
		sshClient.Close()
		sshConn.Close()
	}()

	cs.sendIPMIData(sshSession, machineID, c)

	cs.requestPTY(sshSession)

	done := make(chan bool)

	cs.redirectIO(s, sshSession, done)

	err = sshSession.Start("bash")
	if err != nil {
		cs.log.Sugar().Fatal(err)
		os.Exit(-1)
	}

	// wait till connection is closed
	<-done
}

func (cs *consoleServer) redirectIO(callerSSHSession ssh.Session, machineSSHSession *gossh.Session, done chan<- bool) {
	stdin, err := machineSSHSession.StdinPipe()
	if err != nil {
		cs.log.Sugar().Error("Unable to fetch stdin for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(stdin, callerSSHSession)
			if err != nil && err != io.EOF {
				cs.log.Sugar().Error("Failed to copy caller stdin to machine", "error", err)
			}

			done <- true
		}()
	}

	stdout, err := machineSSHSession.StdoutPipe()
	if err != nil {
		cs.log.Sugar().Error("Unable to fetch stdout for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stdout)
			if err != nil && err != io.EOF {
				cs.log.Sugar().Error("Failed to copy machine stdout to caller", "error", err)
			}

			done <- true
		}()
	}

	stderr, err := machineSSHSession.StderrPipe()
	if err != nil {
		cs.log.Sugar().Error("Unable to fetch stderr for SSH session", "error", err)
	} else {
		go func() {
			_, err = io.Copy(callerSSHSession, stderr)
			if err != nil && err != io.EOF {
				cs.log.Sugar().Error("Failed to copy machine stderr to caller", "error", err)
			}

			done <- true
		}()
	}
}

func (cs *consoleServer) requestPTY(sshSession *gossh.Session) {
	modes := gossh.TerminalModes{
		gossh.ECHO:          0,     // disable echoing
		gossh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		gossh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := sshSession.RequestPty("xterm", 80, 40, modes); err != nil {
		cs.log.Sugar().Error("Failed to request PTY", "error", err)
	}
}

func (cs *consoleServer) connectSSH(tcpConn *tls.Conn, machineID string) (gossh.Conn, *gossh.Client, *gossh.Session) {
	sshConfig := &gossh.ClientConfig{
		User:            machineID,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(tcpConn, cs.spec.BMCReverseProxyAddress, sshConfig)
	if err != nil {
		cs.log.Sugar().Fatal(err)
		os.Exit(-1)
	}

	sshClient := gossh.NewClient(sshConn, chans, reqs)

	sshSession, err := sshClient.NewSession()
	if err != nil {
		cs.log.Sugar().Fatal(err)
		os.Exit(-1)
	}

	return sshConn, sshClient, sshSession
}

func (cs *consoleServer) getConsole(machineID string) (string, error) {
	c, ok := cs.consoles.Load(machineID)
	if !ok {
		return "", fmt.Errorf("unable to fetch requested machine %q", machineID)
	}
	if c == nil {
		cs.log.Sugar().Error("requested machine console is nil", "machineID", machineID)
		return "", fmt.Errorf("no console available for machine %q", machineID)
	}

	return c.(string), nil
}

func (cs *consoleServer) connectToManagementNetwork() *tls.Conn {
	cert, err := tls.LoadX509KeyPair("/client.crt", "/client.pem")
	if err != nil {
		cs.log.Sugar().Error(err)
	}

	caCert, err := ioutil.ReadFile("/ca.crt")
	if err != nil {
		cs.log.Sugar().Error(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}
	tlsConfig.BuildNameToCertificate()

	tcpConn, err := tls.Dial("tcp", cs.spec.BMCReverseProxyAddress, tlsConfig)
	if err != nil {
		cs.log.Sugar().Error("TCP Dial failed", "address", cs.spec.BMCReverseProxyAddress, "error", err)
		return nil
	}
	cs.log.Sugar().Info("Connected to: ", tcpConn.RemoteAddr())

	return tcpConn
}

func (cs *consoleServer) sendIPMIData(sshSession *gossh.Session, machineID, machineCIDR string) {
	var metalIPMI *models.MetalIPMI
	if cs.spec.DevMode() {
		user := "ADMIN" //TODO
		pw := "ADMIN"
		metalIPMI = &models.MetalIPMI{
			Address:  &machineCIDR,
			User:     &user,
			Password: &pw,
		}
	} else {
		var err error
		metalIPMI, err = cs.getIPMIData(machineID)
		if err != nil {
			cs.log.Sugar().Fatal("Failed to fetch IPMI data from Metal API", "machineID", machineID, "error", err)
			os.Exit(-1)
		}
	}

	ipmiData, err := metalIPMI.MarshalBinary()
	if err != nil {
		cs.log.Sugar().Fatal("Failed to marshal MetalIPMI", "error", err)
		os.Exit(-1)
	}

	err = sshSession.Setenv("LC_IPMI_DATA", string(ipmiData))
	if err != nil {
		cs.log.Sugar().Fatal("Failed to send IPMI data to BMC proxy", "error", err)
		os.Exit(-1)
	}
}

func (cs *consoleServer) authHandler(ctx ssh.Context, publickey ssh.PublicKey) bool {
	machineID := ctx.User()
	cs.log.Sugar().Info("authHandler", "machineID", machineID, "publickey", publickey)
	knownAuthorizedKeys, err := cs.getAuthorizedKeysForMachine(machineID)
	if err != nil {
		cs.log.Sugar().Error("authHandler no authorized_keys found", "machineID", machineID, "error", err)
		return false
	}
	for _, key := range knownAuthorizedKeys {
		cs.log.Sugar().Info("authHandler", "machineID", machineID, "authorized_key", key)
		same := ssh.KeysEqual(publickey, key)
		if same {
			return true
		}
	}
	cs.log.Sugar().Warn("authHandler no matching authorized_key found", "machineID", machineID)
	return false
}

func (cs *consoleServer) getAuthorizedKeysForMachine(machineID string) ([]ssh.PublicKey, error) {
	var m *models.MetalMachine
	if cs.spec.DevMode() {
		bb, err := ioutil.ReadFile(cs.spec.PublicKey)
		if err != nil {
			cs.log.Sugar().Error("unable to read public key", "file", cs.spec.PublicKey)
			return nil, err
		}
		m = &models.MetalMachine{
			Allocation: &models.MetalMachineAllocation{
				Cidr: &machineID,
				SSHPubKeys: []string{
					string(bb),
				},
			},
		}
	} else {
		var err error
		m, err = cs.getMachine(machineID)
		if err != nil {
			cs.log.Sugar().Error("unable to fetch requested machine", "machineID", machineID, "error", err)
			return nil, err
		}
		if m == nil {
			cs.log.Sugar().Error("requested machine is nil", "machineID", machineID)
			return nil, err
		}
	}

	cs.consoles.Store(machineID, *m.Allocation.Cidr)

	var pubKeys []ssh.PublicKey
	for _, key := range m.Allocation.SSHPubKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, errors.Wrap(err, "error parsing public key")
		}
		pubKeys = append(pubKeys, pubKey)
	}

	return pubKeys, nil
}

func (cs *consoleServer) publicKeyFromPrivateKeyFile(file string) gossh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		cs.log.Sugar().Error(err)
		return nil
	}

	privKey, err := gossh.ParsePrivateKey(buffer)
	if err != nil {
		cs.log.Sugar().Error(err)
		return nil
	}
	return gossh.PublicKeys(privKey)
}

func loadHostKey() (gossh.Signer, error) {
	bb, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(bb)
}

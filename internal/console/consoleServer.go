package console

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	metalgo "github.com/metal-pod/metal-go"
	"io"
	"io/ioutil"
	"runtime"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
)

type consoleServer struct {
	log           *zap.Logger
	machineClient *metalgo.Driver
	spec          *Specification
	mutex         sync.RWMutex
	ips           *sync.Map
}

func NewServer(log *zap.Logger, spec *Specification) (*consoleServer, error) {
	client, err := newMachineClient(spec.MetalAPIURL, spec.HMACKey)
	if err != nil {
		return nil, err
	}
	return &consoleServer{
		log:           log,
		machineClient: client,
		spec:          spec,
		ips:           &sync.Map{},
		mutex:         sync.RWMutex{},
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
		cs.log.Sugar().Fatal("cannot load host key", "error", err)
		runtime.Goexit()
	}
	s.AddHostKey(hostKey)

	cs.log.Sugar().Info("starting ssh server", "port", cs.spec.Port)
	cs.log.Sugar().Fatal(s.ListenAndServe())
}

func (cs *consoleServer) sessionHandler(s ssh.Session) {
	machineID := s.User()

	ip, err := cs.getIP(machineID)
	if err != nil {
		cs.log.Sugar().Fatal("failed to get console", "machineID", machineID, "error", err)
		s.Exit(1)
		return
	}
	defer cs.ips.Delete(machineID)

	m, err := cs.getMachine(machineID)
	if err != nil {
		cs.log.Sugar().Error("unable to fetch requested machine", "machineID", machineID, "error", err)
		s.Exit(1)
		return
	}

	defer s.Exit(0)

	mgmtServiceAddress := m.Partition.Mgmtserviceaddress

	if cs.spec.DevMode() {
		mgmtServiceAddress = cs.spec.BmcReverseProxyAddress
	}

	tcpConn := cs.connectToManagementNetwork(mgmtServiceAddress)
	defer tcpConn.Close()

	sshConn, sshClient, sshSession := cs.connectSSH(tcpConn, mgmtServiceAddress, machineID)
	defer func() {
		sshSession.Close()
		sshClient.Close()
		sshConn.Close()
	}()

	cs.sendIPMIData(sshSession, machineID, ip)

	cs.requestPTY(sshSession)

	done := make(chan bool)

	cs.redirectIO(s, sshSession, done)

	err = sshSession.Start("bash")
	if err != nil {
		cs.log.Sugar().Fatal(err)
		runtime.Goexit()
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
		gossh.ECHO:          0,      // disable echoing
		gossh.TTY_OP_ISPEED: 115200, // input speed in baud
		gossh.TTY_OP_OSPEED: 115200, // output speed in baud
	}

	if err := sshSession.RequestPty("xterm", 80, 40, modes); err != nil {
		cs.log.Sugar().Error("Failed to request PTY", "error", err)
	}
}

func (cs *consoleServer) connectSSH(tcpConn *tls.Conn, mgmtServiceAddress, machineID string) (gossh.Conn, *gossh.Client, *gossh.Session) {
	sshConfig := &gossh.ClientConfig{
		User:            machineID,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(tcpConn, mgmtServiceAddress, sshConfig)
	if err != nil {
		cs.log.Sugar().Fatal(err)
		runtime.Goexit()
	}

	sshClient := gossh.NewClient(sshConn, chans, reqs)

	sshSession, err := sshClient.NewSession()
	if err != nil {
		cs.log.Sugar().Fatal(err)
		runtime.Goexit()
	}

	return sshConn, sshClient, sshSession
}

func (cs *consoleServer) getIP(machineID string) (string, error) {
	ip, ok := cs.ips.Load(machineID)
	if !ok {
		return "", fmt.Errorf("unable to fetch IP of machine %q", machineID)
	}
	if ip == nil {
		cs.log.Sugar().Error("requested machine IP is nil", "machineID", machineID)
		return "", fmt.Errorf("machine %q not addressable", machineID)
	}

	return ip.(string), nil
}

func (cs *consoleServer) connectToManagementNetwork(mgmtServiceAddress string) *tls.Conn {
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

	tcpConn, err := tls.Dial("tcp", mgmtServiceAddress, tlsConfig)
	if err != nil {
		cs.log.Sugar().Error("TCP Dial failed", "address", mgmtServiceAddress, "error", err)
		return nil
	}
	cs.log.Sugar().Info("Connected to: ", tcpConn.RemoteAddr())

	return tcpConn
}

func (cs *consoleServer) sendIPMIData(sshSession *gossh.Session, machineID, machineIP string) {
	metalIPMI, err := cs.getIPMIData(machineID)
	if err != nil {
		cs.log.Sugar().Fatal("Failed to fetch IPMI data from Metal API", "machineID", machineID, "error", err)
		runtime.Goexit()
	}

	ipmiData, err := metalIPMI.MarshalBinary()
	if err != nil {
		cs.log.Sugar().Fatal("Failed to marshal MetalIPMI", "error", err)
		runtime.Goexit()
	}

	err = sshSession.Setenv("LC_IPMI_DATA", string(ipmiData))
	if err != nil {
		cs.log.Sugar().Fatal("Failed to send IPMI data to BMC proxy", "error", err)
		runtime.Goexit()
	}
}

func (cs *consoleServer) authHandler(ctx ssh.Context, publicKey ssh.PublicKey) bool {
	machineID := ctx.User()
	cs.log.Sugar().Info("authHandler", "publicKey", publicKey)
	knownAuthorizedKeys, err := cs.getAuthorizedKeysForMachine(machineID)
	if err != nil {
		cs.log.Sugar().Error("no authorized keys found", "machineID", machineID, "error", err)
		return false
	}
	for _, key := range knownAuthorizedKeys {
		cs.log.Sugar().Info("authHandler", "machineID", machineID, "authorizedKey", key)
		same := ssh.KeysEqual(publicKey, key)
		if same {
			return true
		}
	}
	cs.log.Sugar().Warn("no matching authorized key found", "machineID", machineID)
	return true //TODO return false
}

func (cs *consoleServer) getAuthorizedKeysForMachine(machineID string) ([]ssh.PublicKey, error) {
	resp, err := cs.getMachine(machineID)
	if err != nil {
		cs.log.Sugar().Error("unable to fetch requested machine", "machineID", machineID, "error", err)
		return nil, err
	}
	if resp == nil {
		cs.log.Sugar().Error("requested machine is nil", "machineID", machineID)
		return nil, err
	}

	if cs.spec.DevMode() {
		bb, err := ioutil.ReadFile(cs.spec.PublicKey)
		if err != nil {
			cs.log.Sugar().Error("unable to read public key", "file", cs.spec.PublicKey)
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
		return nil, fmt.Errorf("unable to detect private IP of machine:%s", machineID)
	}
	cs.ips.Store(machineID, privateIP)

	var pubKeys []ssh.PublicKey
	for _, key := range resp.Allocation.SSHPubKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, errors.Wrap(err, "error parsing public key")
		}
		pubKeys = append(pubKeys, pubKey)
	}

	return pubKeys, nil
}

func loadHostKey() (gossh.Signer, error) {
	bb, err := ioutil.ReadFile("/host-key")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(bb)
}

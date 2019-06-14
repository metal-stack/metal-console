package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	rt "github.com/go-openapi/runtime"
	"io"
	"io/ioutil"
	"sync"
	"runtime"

	"github.com/go-openapi/strfmt"
	"time"

	"github.com/metal-pod/security"

	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/client/machine"
	"git.f-i-ts.de/cloud-native/metal/metal-console/metal-api/models"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
)

type consoleServer struct {
	log           *zap.Logger
	machineClient *machine.Client
	spec          *Specification
	mutex         sync.RWMutex
	ips           *sync.Map
	hmac          security.HMACAuth
	Auth          rt.ClientAuthInfoWriter
}

func New(log *zap.Logger, spec *Specification) *consoleServer {
	cs := &consoleServer{
		log:           log,
		machineClient: newMachineClient(spec.MetalAPIAddress),
		spec:          spec,
		ips:           &sync.Map{},
		mutex:         sync.RWMutex{},
	}
	cs.InitHMAC(spec.HMACKey)
	return cs
}

func (cs *consoleServer) InitHMAC(hmacKey string) {
	cs.hmac = security.NewHMACAuth("Metal-Edit", []byte(hmacKey))
	cs.Auth = rt.ClientAuthInfoWriterFunc(cs.auther)
}

func (cs *consoleServer) auther(rq rt.ClientRequest, rg strfmt.Registry) error {
	cs.hmac.AddAuthToClientRequest(rq, time.Now())
	return nil
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
	var metalIPMI *models.V1MachineIPMI
	if cs.spec.DevMode() {
		user := "ADMIN" //TODO
		pw := "ADMIN"
		metalIPMI = &models.V1MachineIPMI{
			Address:  &machineIP,
			User:     &user,
			Password: &pw,
		}
	} else {
		var err error
		metalIPMI, err = cs.getIPMIData(machineID)
		if err != nil {
			cs.log.Sugar().Fatal("Failed to fetch IPMI data from Metal API", "machineID", machineID, "error", err)
			runtime.Goexit()
		}
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
	var m *models.V1MachineResponse
	if cs.spec.DevMode() {
		bb, err := ioutil.ReadFile(cs.spec.PublicKey)
		if err != nil {
			cs.log.Sugar().Error("unable to read public key", "file", cs.spec.PublicKey)
			return nil, err
		}
		primary := true
		m = &models.V1MachineResponse{
			Allocation: &models.V1MachineAllocation{
				Networks: []*models.V1MachineNetwork{{Primary: &primary, Ips: []string{machineID}}},
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

	primaryIP := ""
	if m.Allocation != nil {
		for _, nw := range m.Allocation.Networks {
			if *nw.Primary {
				if len(nw.Ips) > 0 {
					primaryIP = nw.Ips[0]
					break
				}
			}
		}
	}
	if primaryIP == "" {
		return nil, fmt.Errorf("unable to detect primary IP of machine:%s", machineID)
	}
	cs.ips.Store(machineID, primaryIP)

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
	bb, err := ioutil.ReadFile("/host-key")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(bb)
}

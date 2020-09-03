package bmcproxy

import (
	"fmt"
	"github.com/metal-stack/go-hal/detect"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/metal-stack/metal-go/api/models"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
)

type bmcProxy struct {
	log  *zap.SugaredLogger
	spec *Specification
}

func New(log *zap.SugaredLogger, spec *Specification) *bmcProxy {
	return &bmcProxy{
		log:  log,
		spec: spec,
	}
}

// Run starts ssh server and listen for console connections.
func (p *bmcProxy) Run() {
	s := &ssh.Server{
		Addr:    fmt.Sprintf(":%d", p.spec.Port),
		Handler: p.sessionHandler,
	}

	hostKey, err := loadHostKey()
	if err != nil {
		p.log.Errorw("cannot load host key", "error", err)
		os.Exit(1)
	}
	s.AddHostKey(hostKey)

	p.log.Infow("starting ssh server", "port", p.spec.Port)
	p.log.Fatal(s.ListenAndServe())
}

func (p *bmcProxy) sessionHandler(s ssh.Session) {
	p.log.Infow("ssh session handler called", "user", s.User(), "env", s.Environ())
	machineID := s.User()
	metalIPMI := p.receiveIPMIData(s)
	p.log.Infow("connection to", "machineID", machineID)
	if metalIPMI == nil {
		p.log.Errorw("failed to receive IPMI data", "machineID", machineID)
		return
	}
	if metalIPMI.Address == nil {
		p.log.Errorw("failed to receive IPMI.Address data", "machineID", machineID)
		return
	}
	_, err := io.WriteString(s, fmt.Sprintf("Connecting to console of %q (%s)\n", machineID, *metalIPMI.Address))
	if err != nil {
		p.log.Warnw("failed to write to console", "machineID", machineID)
	}

	addressParts := strings.Split(*metalIPMI.Address, ":")
	host := addressParts[0]
	port, err := strconv.Atoi(addressParts[1])
	if err != nil {
		p.log.Errorw("invalid port", "port", port, "address", *metalIPMI.Address)
		return
	}

	ob, err := detect.ConnectOutBand(host, port, *metalIPMI.User, *metalIPMI.Password)
	if err != nil {
		p.log.Errorw("failed to out-band connect", "host", host, "port", port, "machineID", machineID, "ipmiuser", *metalIPMI.User)
		return
	}

	err = ob.Console(s)
	if err != nil {
		p.log.Errorw("failed to access console", "machineID", machineID, "error", err)
	}
}

func (p *bmcProxy) receiveIPMIData(s ssh.Session) *models.V1MachineIPMI {
	var ipmiData string
	for i := 0; i < 5; i++ {
		for _, env := range s.Environ() {
			parts := strings.Split(env, "=")
			if len(parts) == 2 && parts[0] == "LC_IPMI_DATA" {
				ipmiData = parts[1]
				break
			}
		}
		if len(ipmiData) > 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if len(ipmiData) == 0 {
		p.log.Error("failed to receive IPMI data")
		return nil
	}

	metalIPMI := &models.V1MachineIPMI{}
	err := metalIPMI.UnmarshalBinary([]byte(ipmiData))
	if err != nil {
		p.log.Errorw("failed to unmarshal received IPMI data", "error", err)
		return nil
	}

	return metalIPMI
}

func loadHostKey() (gossh.Signer, error) {
	bb, err := ioutil.ReadFile("/server-key.pem")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(bb)
}

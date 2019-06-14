package bmcproxy

import (
	"fmt"
	"git.f-i-ts.de/cloud-native/metal/bmc-proxy/metal-api/models"
	"github.com/gliderlabs/ssh"
	"github.com/kr/pty"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type bmcProxy struct {
	log  *zap.Logger
	spec *Specification
}

func New(log *zap.Logger, spec *Specification) *bmcProxy {
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
		p.log.Sugar().Fatal("cannot load host key", "error", err)
		os.Exit(-1)
	}
	s.AddHostKey(hostKey)

	p.log.Sugar().Info("starting ssh server", "port", p.spec.Port)
	p.log.Sugar().Fatal(s.ListenAndServe())
}

func (p *bmcProxy) sessionHandler(s ssh.Session) {
	machineID := s.User()
	metalIPMI := p.receiveIPMIData(s)
	p.log.Sugar().Info("connection to", "machineID", machineID)
	if metalIPMI == nil {
		p.log.Sugar().Fatal("failed to receive IPMI data", "machineID", machineID)
		s.Exit(1)
		return
	}
	io.WriteString(s, fmt.Sprintf("Connecting to console of %q (%s)\n", machineID, *metalIPMI.Address))

	var cmd *exec.Cmd
	if p.spec.DevMode {
		io.WriteString(s, "Exit with '<Ctrl> 5'\n")
		cmd = exec.Command("virsh", "console", *metalIPMI.Address, "--force")
	} else {
		io.WriteString(s, "Exit with '~.'\n")
		addressParts := strings.Split(*metalIPMI.Address, ":")
		host := addressParts[0]
		port := addressParts[1]

		command := "ipmitool"
		args := []string{"-I", "lanplus", "-H", host, "-p", port, "-U", *metalIPMI.User, "-P", *metalIPMI.Password, "sol", "activate"}
		p.log.Sugar().Infow("console", "command", command, "args", args)

		cmd = exec.Command(command, args...)
	}
	cmd.Env = os.Environ()
	ptyReq, winCh, isPty := s.Pty()
	if isPty {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
		f, err := pty.Start(cmd)
		if err != nil {
			p.log.Sugar().Error("Command execution failed", "error", err)
			// do not tell the user what went wrong
			s.Exit(1)
			return
		}

		go func() {
			for win := range winCh {
				setWinSize(f, win.Width, win.Height)
			}
		}()

		done := make(chan bool)

		go func() {
			_, err = io.Copy(f, s) // stdin
			if err != nil && err != io.EOF && !strings.HasSuffix(err.Error(), syscall.EIO.Error()) {
				p.log.Sugar().Error("Failed to copy remote stdin to local", "error", err)
			}
			done <- true
		}()

		go func() {
			_, err = io.Copy(s, f) // stdout
			if err != nil && err != io.EOF && !strings.HasSuffix(err.Error(), syscall.EIO.Error()) {
				p.log.Sugar().Error("Failed to copy local stdout to remote", "error", err)
			}
			done <- true
		}()

		// wait till connection is closed
		<-done

	} else {
		io.WriteString(s, "No PTY requested.\n")
		s.Exit(1)
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
		p.log.Sugar().Error("Failed to receive IPMI data")
		os.Exit(-1)
	}

	metalIPMI := &models.V1MachineIPMI{}
	err := metalIPMI.UnmarshalBinary([]byte(ipmiData))
	if err != nil {
		p.log.Sugar().Error("Failed to unmarshal received IPMI data", "error", err)
		os.Exit(-1)
	}

	return metalIPMI
}

func setWinSize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func loadHostKey() (gossh.Signer, error) {
	privateBytes, err := ioutil.ReadFile("/host-key")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(privateBytes)
}

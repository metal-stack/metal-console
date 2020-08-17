package bmcproxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
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

func New(log *zap.Logger, spec *Specification) *bmcProxy {
	return &bmcProxy{
		log:  log.Sugar(),
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
		p.log.Fatalw("cannot load host key", "error", err)
		os.Exit(1)
	}
	s.AddHostKey(hostKey)

	p.log.Infow("starting ssh server", "port", p.spec.Port)
	p.log.Fatal(s.ListenAndServe())
}

func (p *bmcProxy) sessionHandler(s ssh.Session) {
	machineID := s.User()
	metalIPMI := p.receiveIPMIData(s)
	p.log.Infow("connection to", "machineID", machineID)
	if metalIPMI == nil {
		p.log.Fatalw("failed to receive IPMI data", "machineID", machineID)
		return
	}
	if metalIPMI.Address == nil {
		p.log.Fatalw("failed to receive IPMI.Address data", "machineID", machineID)
		return
	}
	_, err := io.WriteString(s, fmt.Sprintf("Connecting to console of %q (%s)\n", machineID, *metalIPMI.Address))
	if err != nil {
		p.log.Warnw("failed to write to console", "machineID", machineID)
	}

	var cmd *exec.Cmd
	if p.spec.DevMode {
		_, err = io.WriteString(s, "Exit with '<Ctrl> 5'\n")
		if err != nil {
			p.log.Warnw("failed to write to console", "machineID", machineID)
		}
		cmd = exec.Command("virsh", "console", *metalIPMI.Address, "--force")
	} else {
		_, err = io.WriteString(s, "Exit with '~.'\n")
		if err != nil {
			p.log.Warnw("failed to write to console", "machineID", machineID)
		}
		addressParts := strings.Split(*metalIPMI.Address, ":")
		host := addressParts[0]
		port := addressParts[1]

		command := "ipmitool"
		args := []string{"-I", "lanplus", "-H", host, "-p", port, "-U", *metalIPMI.User, "-P", strconv.Quote(*metalIPMI.Password), "sol", "activate"}
		p.log.Infow("console", "command", command, "args", args)

		cmd = exec.Command(command, args...)
	}
	cmd.Env = os.Environ()
	ptyReq, winCh, isPty := s.Pty()
	if isPty {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
		f, err := pty.Start(cmd)
		if err != nil {
			p.log.Errorw("failed to execute command via PTY", "command", cmd.Path, "args", strings.Join(cmd.Args, " "), "error", err)
			err = s.Exit(1)
			if err != nil {
				p.log.Errorw("failed to exit ssh session", "error", err)
			}
			return
		}

		go func() {
			for win := range winCh {
				err := setWinSize(f, win.Width, win.Height)
				if err != nil {
					p.log.Errorw("failed to set window size", "error", err)
				}
			}
		}()

		done := make(chan bool)

		go func() {
			_, err = io.Copy(f, s) // stdin
			if err != nil && err != io.EOF && !strings.HasSuffix(err.Error(), syscall.EIO.Error()) {
				p.log.Errorw("failed to copy remote stdin to local", "error", err)
			}
			done <- true
		}()

		go func() {
			_, err = io.Copy(s, f) // stdout
			if err != nil && err != io.EOF && !strings.HasSuffix(err.Error(), syscall.EIO.Error()) {
				p.log.Errorw("failed to copy local stdout to remote", "error", err)
			}
			done <- true
		}()

		// wait till connection is closed
		<-done

	} else {
		_, err = io.WriteString(s, "No PTY requested.\n")
		if err != nil {
			p.log.Warnw("failed to write to console", "machineID", machineID)
		}
		err = s.Exit(1)
		if err != nil {
			p.log.Errorw("failed to exit ssh session", "error", err)
		}
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
		os.Exit(1)
	}

	metalIPMI := &models.V1MachineIPMI{}
	err := metalIPMI.UnmarshalBinary([]byte(ipmiData))
	if err != nil {
		p.log.Errorw("failed to unmarshal received IPMI data", "error", err)
		os.Exit(1)
	}

	return metalIPMI
}

func setWinSize(f *os.File, w, h int) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
	return err
}

func loadHostKey() (gossh.Signer, error) {
	bb, err := ioutil.ReadFile("/host-key.pem")
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}
	return gossh.ParsePrivateKey(bb)
}

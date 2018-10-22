package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/gliderlabs/ssh"
	log "github.com/inconshreveable/log15"
	"github.com/kr/pty"
	gossh "golang.org/x/crypto/ssh"
)

type Console struct {
	Spec *Specification
}

// Run start ssh server and listen for console connections.
func (c *Console) Run() error {
	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", c.Spec.Port),
		Handler:          c.sessionHandler,
		PublicKeyHandler: c.authHandler,
	}

	hostkey, err := loadHostKey()
	if err != nil {
		return err
	}
	s.AddHostKey(hostkey)

	log.Info("starting ssh server", "port", c.Spec.Port)
	return s.ListenAndServe()
}

func (c *Console) sessionHandler(s ssh.Session) {
	io.WriteString(s, fmt.Sprintf("connecting to console of %s\n", s.User()))
	io.WriteString(s, fmt.Sprintf("Exit with <STRG>5\n"))

	metalDevice, err := getDevice(c.Spec.MetalAPIUrl, s.User())
	if err != nil {
		log.Error("unable to fetch requested device", "device", s.User(), "error", err)
		s.Exit(1)
	}
	if metalDevice == nil {
		log.Error("requested device is nil", "device", s.User())
		s.Exit(1)
	}

	// TODO must be different for metal-lab devices and real hardware.
	// for virsh must be in the form virsh --connect <libvirt uri> console <device>
	cmd := exec.Command("virsh", "console", s.User())
	cmd.Env = os.Environ()
	ptyReq, winCh, isPty := s.Pty()
	if isPty {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
		f, err := pty.Start(cmd)
		if err != nil {
			log.Error("command execution failed", "error", err)
			// do not tell the user what went wrong
			s.Exit(1)
		}
		go func() {
			for win := range winCh {
				setWinsize(f, win.Width, win.Height)
			}
		}()
		go func() {
			io.Copy(f, s) // stdin
		}()
		io.Copy(s, f) // stdout
	} else {
		io.WriteString(s, "No PTY requested.\n")
		s.Exit(1)
	}
}

func (c *Console) authHandler(ctx ssh.Context, publickey ssh.PublicKey) bool {
	device := ctx.User()
	log.Info("authHandler", "device", device, "publickey", publickey)
	knownAuthorizedKeys, err := c.getAuthorizedKeysforDevice(device)
	if err != nil {
		log.Error("authHandler no authorized_keys found", "device", device, "error", err)
		return false
	}
	for _, key := range knownAuthorizedKeys {
		log.Info("authHandler", "device", device, "authorized_key", key)
		same := ssh.KeysEqual(publickey, key)
		if same {
			return true
		}
	}
	log.Warn("authHandler no matching authorized_key found", "device", device)
	return false
}

func loadHostKey() (gossh.Signer, error) {
	privateBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %v", err)
	}
	return gossh.ParsePrivateKey(privateBytes)
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func (c *Console) getAuthorizedKeysforDevice(device string) ([]ssh.PublicKey, error) {
	metalDevice, err := getDevice(c.Spec.MetalAPIUrl, device)
	result := []ssh.PublicKey{}
	if err != nil {
		log.Error("unable to fetch requested device", "device", device, "error", err)
		return result, err
	}
	if metalDevice == nil {
		log.Error("requested device is nil", "device", device)
		return result, err
	}
	pubkey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(*metalDevice.SSHPubKey))
	if err != nil {
		return result, fmt.Errorf("error parsing public key:%v", err)
	}
	// TODO metal-api must have multiple pubkeys per device
	result = append(result, pubkey)
	return result, nil
}

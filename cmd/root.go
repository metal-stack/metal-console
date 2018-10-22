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

// Run start ssh server and listen for console connections.
func Run(port int) error {

	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", port),
		Handler:          sessionHandler,
		PublicKeyHandler: authHandler,
	}

	hostkey, err := loadHostKey()
	if err != nil {
		return err
	}
	s.AddHostKey(hostkey)

	log.Info("starting ssh server", "port", port)
	return s.ListenAndServe()
}

func sessionHandler(s ssh.Session) {
	io.WriteString(s, fmt.Sprintf("connecting to console of %s\n", s.User()))
	io.WriteString(s, fmt.Sprintf("Exit with <STRG>5\n"))

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

func authHandler(ctx ssh.Context, publickey ssh.PublicKey) bool {
	device := ctx.User()
	log.Info("authHandler", "device", device, "publickey", publickey)
	knownAuthorizedKeys := getAuthorizedKeysforDevice(device)
	for _, key := range knownAuthorizedKeys {
		same := ssh.KeysEqual(publickey, key)
		if same {
			return true
		}
	}
	return true
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

func getAuthorizedKeysforDevice(device string) []ssh.PublicKey {
	// TODO ask metal-api for publickey by user (device)
	return nil
}

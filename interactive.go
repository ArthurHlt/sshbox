package sshbox

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/moby/term"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/ArthurHlt/sshbox/sigwinch"
)

type TTYRequest int

const (
	RequestTTYAuto TTYRequest = iota
	RequestTTYNo
	RequestTTYYes
	RequestTTYForce
)

type InteractiveSSH struct {
	sshBox  *SSHBox
	session *ssh.Session
}

func NewInteractiveSSH(sshBox *SSHBox) *InteractiveSSH {
	return &InteractiveSSH{sshBox: sshBox}
}

func (c *InteractiveSSH) startInteractive(commands []string, subSystem string, terminalRequest TTYRequest, sessOpts ...SSHSessionOptions) error {
	var err error
	c.session, err = c.sshBox.SSHClient().NewSession()
	if err != nil {
		return fmt.Errorf("SSH session allocation failed: %s", err.Error())
	}
	defer c.session.Close()

	for _, opt := range sessOpts {
		err := opt(c.session)
		if err != nil {
			return fmt.Errorf("SSH session option failed: %s", err.Error())
		}
	}

	stdin, stdout, stderr := term.StdStreams()

	inPipe, err := c.session.StdinPipe()
	if err != nil {
		return err
	}

	outPipe, err := c.session.StdoutPipe()
	if err != nil {
		return err
	}

	errPipe, err := c.session.StderrPipe()
	if err != nil {
		return err
	}

	stdinFd, stdinIsTerminal := term.GetFdInfo(stdin)
	stdoutFd, stdoutIsTerminal := term.GetFdInfo(stdout)

	if c.shouldAllocateTerminal(commands, terminalRequest, stdinIsTerminal) {
		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 115200,
			ssh.TTY_OP_OSPEED: 115200,
		}

		width, height := c.getWindowDimensions(stdoutFd)

		err = c.session.RequestPty(c.terminalType(), height, width, modes)
		if err != nil {
			return err
		}

		var state *term.State
		state, err = term.SetRawTerminal(stdinFd)
		if err == nil {
			defer func() {
				err := term.RestoreTerminal(stdinFd, state)
				if err != nil {
					log.Errorln("restore terminal", err)
				}
			}()
		}
	}
	if subSystem != "" {
		err = c.session.RequestSubsystem(subSystem)
	} else if len(commands) > 0 {
		cmd := strings.Join(commands, " ")
		err = c.session.Start(cmd)
	} else {
		err = c.session.Shell()
	}
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func(dest io.WriteCloser, src io.Reader) {
		_, err := io.Copy(dest, src)
		if err != nil {
			log.Errorln("copy and close:", err)
		}
		_ = dest.Close()
	}(inPipe, stdin)

	copyAndDone := func(wg *sync.WaitGroup, dest io.Writer, src io.Reader) {
		defer wg.Done()
		_, err := io.Copy(dest, src)
		if err != nil {
			log.Errorln("copy and done:", err)
		}
	}
	go copyAndDone(wg, stdout, outPipe)
	go copyAndDone(wg, stderr, errPipe)

	if stdoutIsTerminal {
		resized := make(chan os.Signal, 16)

		if runtime.GOOS == "windows" {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()

			go func() {
				for range ticker.C {
					resized <- syscall.Signal(-1)
				}
				close(resized)
			}()
		} else {
			signal.Notify(resized, sigwinch.SIGWINCH())
			defer func() { signal.Stop(resized); close(resized) }()
		}

		go c.resize(resized, stdoutFd)
	}
	result := c.session.Wait()
	wg.Wait()
	return result
}

func (c *InteractiveSSH) InteractiveSessionSubSystem(subsystem string, terminalRequest TTYRequest, sessOpts ...SSHSessionOptions) error {
	return c.startInteractive(nil, subsystem, terminalRequest, sessOpts...)
}

func (c *InteractiveSSH) InteractiveSession(commands []string, terminalRequest TTYRequest, sessOpts ...SSHSessionOptions) error {
	return c.startInteractive(commands, "", terminalRequest, sessOpts...)
}

func (c *InteractiveSSH) Interactive(sessOpts ...SSHSessionOptions) error {
	return c.InteractiveSession([]string{}, RequestTTYAuto, sessOpts...)
}

func (c *InteractiveSSH) InteractiveSubSystem(subsystem string, sessOpts ...SSHSessionOptions) error {
	return c.InteractiveSessionSubSystem(subsystem, RequestTTYAuto, sessOpts...)
}

func (c *InteractiveSSH) RunCmd(cmd []string, sessOpts ...SSHSessionOptions) error {
	return c.InteractiveSession(cmd, RequestTTYAuto, sessOpts...)
}

func (c *InteractiveSSH) Stop() error {
	return c.session.Close()
}

func (c *InteractiveSSH) getWindowDimensions(terminalFd uintptr) (width int, height int) {
	winSize, err := term.GetWinsize(terminalFd)
	if err != nil {
		winSize = &term.Winsize{
			Width:  80,
			Height: 43,
		}
	}

	return int(winSize.Width), int(winSize.Height)
}

func (c *InteractiveSSH) resize(resized <-chan os.Signal, terminalFd uintptr) {
	type resizeMessage struct {
		Width       uint32
		Height      uint32
		PixelWidth  uint32
		PixelHeight uint32
	}

	var previousWidth, previousHeight int

	for range resized {
		width, height := c.getWindowDimensions(terminalFd)

		if width == previousWidth && height == previousHeight {
			continue
		}

		message := resizeMessage{
			Width:  uint32(width),
			Height: uint32(height),
		}

		_, err := c.session.SendRequest("window-change", false, ssh.Marshal(message))
		if err != nil {
			log.Errorln("window-change:", err)
		}

		previousWidth = width
		previousHeight = height
	}
}

func (c *InteractiveSSH) terminalType() string {
	t := os.Getenv("TERM")
	if t == "" {
		t = "xterm"
	}
	return t
}

func (c *InteractiveSSH) shouldAllocateTerminal(commands []string, terminalRequest TTYRequest, stdinIsTerminal bool) bool {
	switch terminalRequest {
	case RequestTTYForce:
		return true
	case RequestTTYNo:
		return false
	case RequestTTYYes:
		return stdinIsTerminal
	case RequestTTYAuto:
		return len(commands) == 0 && stdinIsTerminal
	default:
		return false
	}
}

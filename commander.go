package sshbox

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

// CommanderSSH let you run commands on a remote host and getting the output back
// It will create a session each time a command is run which mean that context is not persisted between commands
type CommanderSSH struct {
	sshBox *SSHBox
}

func NewCommanderSSH(sshBox *SSHBox) *CommanderSSH {
	return &CommanderSSH{
		sshBox: sshBox,
	}
}

func (c *CommanderSSH) Run(cmd string, opts ...SSHSessionOptions) (stdout []byte, stderr []byte, err error) {
	sess, err := MakeSessionNoTerminal(c.sshBox.SSHClient(), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make sessions: %s", err)
	}
	defer sess.Close()
	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}
	sess.Stdout = stdoutBuffer
	sess.Stderr = stderrBuffer
	err = sess.Run(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to run command: %s", err)
	}
	return stdoutBuffer.Bytes(), stderrBuffer.Bytes(), nil
}

func (c *CommanderSSH) CombinedOutput(cmd string, opts ...SSHSessionOptions) ([]byte, error) {
	sess, err := MakeSessionNoTerminal(c.sshBox.SSHClient(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to make sessions: %s", err)
	}
	defer sess.Close()
	return sess.CombinedOutput(cmd)
}

type ExpectPromptMatcher func(line []byte) bool

func DefaultMatcher(line []byte) bool {
	return strings.Contains(string(line), "$ ")
}

// CommanderSession let you run multiple commands on a remote host and getting the output back on a single session
// which means that context is persisted between commands but output is buffered and split by a matcher which is often the prompt
type CommanderSession struct {
	session *ssh.Session
	matcher ExpectPromptMatcher
	output  *singleWriter
	stdin   io.Writer
}

// NewCommanderSession creates a new commander session
// matcher can be nil and it will use the DefaultMatcher as matcher
func NewCommanderSession(client *ssh.Client, matcher ExpectPromptMatcher, opts ...SSHSessionOptions) (*CommanderSession, error) {
	sess, err := MakeSessionNoTerminal(client, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to make sessions: %s", err)
	}
	inPipe, err := sess.StdinPipe()
	if err != nil {
		return nil, err
	}

	outPipe, err := sess.StdoutPipe()
	if err != nil {
		return nil, err
	}

	errPipe, err := sess.StderrPipe()
	if err != nil {
		return nil, err
	}
	output := &singleWriter{}
	copyAndDone := func(dest io.Writer, src io.Reader) {
		_, err := io.Copy(dest, src)
		if err != nil {
			log.Errorln("copy and done:", err)
		}
	}
	go copyAndDone(output, outPipe)
	go copyAndDone(output, errPipe)
	err = sess.Shell()
	if err != nil {
		return nil, err
	}
	if matcher == nil {
		matcher = DefaultMatcher
	}
	cmderSess := &CommanderSession{
		session: sess,
		matcher: matcher,
		output:  output,
		stdin:   inPipe,
	}
	_, err = cmderSess.waitUntil()
	if err != nil {
		return nil, err
	}
	return cmderSess, nil
}

func (c *CommanderSession) SetMatcher(matcher ExpectPromptMatcher) {
	c.matcher = matcher
}

func (c *CommanderSession) Run(cmd string) ([]byte, error) {
	c.output.Reset()
	_, err := fmt.Fprintf(c.stdin, "%s\n", cmd)
	if err != nil {
		return nil, err
	}
	return c.waitUntil()
}

func (c *CommanderSession) waitUntil() ([]byte, error) {
	for {
		splitLines := bytes.Split(c.output.b.Bytes(), []byte{'\n'})
		lastLine := splitLines[len(splitLines)-1]
		if !c.matcher(lastLine) {
			continue
		}
		lines := splitLines[:len(splitLines)-1]
		return c.dropCR(bytes.Join(lines, []byte{'\n'})), nil
	}
	return c.output.b.Bytes(), nil
}

func (c *CommanderSession) dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}

func (c *CommanderSession) Close() error {
	return c.session.Close()
}

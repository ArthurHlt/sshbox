package sshbox

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

var sanitizeRE = []*regexp.Regexp{
	regexp.MustCompile(`\x1b\[\?1h\x1b=`),
	regexp.MustCompile(`\x08.`),
	regexp.MustCompile(`\x1b\[m`),
}

var errorOutputRE = regexp.MustCompile(`(?i)(error|bad|invalid|unknown)`)

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

func DefaultPromptMatcher(line []byte) bool {
	return strings.Contains(string(line), "$ ")
}

func DefaultErrorMatcher(content []byte) bool {
	return errorOutputRE.Match(content)
}

func DefaultSanitizePromptLine(line []byte) []byte {
	return make([]byte, 0)
}

// CommanderSession let you run multiple commands on a remote host and getting the output back on a single session
// which means that context is persisted between commands but output is buffered and split by a promptMatcher which is often the prompt
type CommanderSession struct {
	session            *ssh.Session
	promptMatcher      func(line []byte) bool
	sanitizePromptLine func(line []byte) []byte
	errorMatcher       func(content []byte) bool
	separator          []byte
	output             *singleWriter
	stdin              io.Writer
	sessOpts           []SSHSessionOptions
	subSystem          string
}

type commanderSessionOptions func(*CommanderSession) error

// WithErrorMatcher option to set the prompt matcher
func WithPromptMatcher(promptMatcher func(line []byte) bool) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.promptMatcher = promptMatcher
		return nil
	}
}

// WithErrorMatcher option to set the error matcher
func WithErrorMatcher(errorMatcher func(content []byte) bool) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.errorMatcher = errorMatcher
		return nil
	}
}

// WithSessionOptions option to add options to the session
func WithSessionOptions(opts ...SSHSessionOptions) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.sessOpts = opts
		return nil
	}
}

// WithSubSystem option to use subsystem instead of shell
func WithSubSystem(subsystem string) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.subSystem = subsystem
		return nil
	}
}

// WithSeparator option to set a separator content
func WithSeparator(separator []byte) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.separator = separator
		return nil
	}
}

func WithSanitizePromptLine(sanitizePromptLine func(line []byte) []byte) commanderSessionOptions {
	return func(c *CommanderSession) error {
		c.sanitizePromptLine = sanitizePromptLine
		return nil
	}
}

// NewCommanderSession creates a new commander session
func NewCommanderSession(client *ssh.Client, opts ...commanderSessionOptions) (*CommanderSession, error) {
	cmderSess := &CommanderSession{}
	for _, opt := range opts {
		err := opt(cmderSess)
		if err != nil {
			return nil, err
		}
	}
	if len(cmderSess.separator) == 0 {
		cmderSess.separator = []byte("\n")
	}
	sess, err := MakeSessionNoTerminal(client, cmderSess.sessOpts...)
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
	if cmderSess.subSystem != "" {
		err = sess.RequestSubsystem(cmderSess.subSystem)
		if err != nil {
			return nil, err
		}
	} else {
		err = sess.Shell()
		if err != nil {
			return nil, err
		}
	}

	if cmderSess.promptMatcher == nil {
		cmderSess.promptMatcher = DefaultPromptMatcher
	}
	if cmderSess.errorMatcher == nil {
		cmderSess.errorMatcher = DefaultErrorMatcher
	}
	if cmderSess.sanitizePromptLine == nil {
		cmderSess.sanitizePromptLine = DefaultSanitizePromptLine
	}
	cmderSess.session = sess
	cmderSess.output = output
	cmderSess.stdin = inPipe
	_, err = cmderSess.waitUntil()
	if err != nil {
		return nil, err
	}
	return cmderSess, nil
}

func (c *CommanderSession) SetMatcher(matcher func(line []byte) bool) {
	c.promptMatcher = matcher
}

func (c *CommanderSession) Run(cmd string) ([]byte, error) {
	c.output.Reset()
	_, err := fmt.Fprintf(c.stdin, "%s\n", cmd)
	if err != nil {
		return nil, err
	}
	result, err := c.waitUntil()
	if err != nil {
		return nil, err
	}
	if c.errorMatcher(result) {
		return nil, errTerminalError(result)
	}
	return result, nil
}

func (c *CommanderSession) waitUntil() ([]byte, error) {
	for {
		splitLines := bytes.Split(c.output.b.Bytes(), c.separator)
		lastLine := splitLines[len(splitLines)-1]
		if !c.promptMatcher(lastLine) {
			continue
		}
		lines := splitLines[:len(splitLines)-1]
		lastLineSan := c.sanitizePromptLine(lastLine)
		if len(lastLineSan) > 0 {
			lines = append(lines, lastLineSan)
		}
		return c.sanitize(bytes.Join(lines, c.separator)), nil
	}
	return c.output.b.Bytes(), nil
}

func (c *CommanderSession) sanitize(line []byte) []byte {
	for _, re := range sanitizeRE {
		re.ReplaceAll(line, []byte(""))
	}
	return c.dropCR(line)
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

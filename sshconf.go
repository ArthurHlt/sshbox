package sshbox

import (
	"fmt"
	"net"
	"os"
	osuser "os/user"
)

type SSHConf struct {
	SSHUri             string
	User               string
	Password           string
	PrivateKey         string
	Passphrase         string
	HostKeyFingerprint string
	SSHAuthSock        *string
	NoSSHAgent         bool
}

func (c *SSHConf) CheckAndFill() error {
	if c.SSHUri == "" {
		return fmt.Errorf("You must set an ssh uri")
	}
	user, _ := osuser.Current()
	if c.User == "" {
		c.User = user.Name
	}
	if c.User == "" {
		c.User = user.Username
	}
	if c.User == "" {
		c.User = "root"
	}

	_, _, err := net.SplitHostPort(c.SSHUri)
	if err != nil {
		c.SSHUri += ":22"
	}
	if c.NoSSHAgent {
		emptyString := ""
		c.SSHAuthSock = &emptyString
	}
	if c.SSHAuthSock == nil && os.Getenv("SSH_AUTH_SOCK") != "" {
		sock := os.Getenv("SSH_AUTH_SOCK")
		c.SSHAuthSock = &sock
	}
	return nil
}

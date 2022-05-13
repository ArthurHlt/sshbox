package sshbox

import "golang.org/x/crypto/ssh"

type SSHSessionOptions func(session *ssh.Session) error

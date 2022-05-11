package sshbox

import "golang.org/x/crypto/ssh"

type SSHSessionOptions func(session *ssh.Session) error

func SetSubsystem(subsystem string) SSHSessionOptions {
	return func(session *ssh.Session) error {
		return session.RequestSubsystem(subsystem)
	}
}

package sshbox

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	md5FingerprintLength          = 47 // inclusive of space between bytes
	hexSha1FingerprintLength      = 59 // inclusive of space between bytes
	base64Sha256FingerprintLength = 43
)

type SshClientFactory func(conf SSHConf) (*ssh.Client, error)

func DefaultSshClientFactory(conf SSHConf) (*ssh.Client, error) {
	var hostKeyCallback ssh.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	if conf.HostKeyFingerprint != "" {
		hostKeyCallback = fingerprintCallback(conf.HostKeyFingerprint)
	}
	authMethods := make([]ssh.AuthMethod, 0)
	sshAuthSock := ""
	if conf.SSHAuthSock != nil && !conf.NoSSHAgent {
		sshAuthSock = *conf.SSHAuthSock
	}
	if sshAgent, err := net.Dial("unix", sshAuthSock); err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}
	if conf.Password != "" {
		authMethods = append(authMethods, ssh.Password(conf.Password))
	}

	if conf.PrivateKey != "" {
		pubKeys, err := NewPublicKeysFromFile(conf.PrivateKey, conf.Passphrase)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(pubKeys.Signer))
	}

	host, port, err := net.SplitHostPort(conf.Host)
	if err != nil {
		host = conf.Host
		port = "22"
	}

	serverConn, err := ssh.Dial("tcp", host+":"+port, &ssh.ClientConfig{
		User:            conf.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	})

	if err != nil {
		return nil, err
	}
	return serverConn, nil
}

func md5Fingerprint(key ssh.PublicKey) string {
	sum := md5.Sum(key.Marshal())
	return strings.Replace(fmt.Sprintf("% x", sum), " ", ":", -1)
}

func hexSha1Fingerprint(key ssh.PublicKey) string {
	sum := sha1.Sum(key.Marshal())
	return strings.Replace(fmt.Sprintf("% x", sum), " ", ":", -1)
}

func base64Sha256Fingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func fingerprintCallback(expectedFingerprint string) ssh.HostKeyCallback {

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		var fingerprint string

		switch len(expectedFingerprint) {
		case base64Sha256FingerprintLength:
			fingerprint = base64Sha256Fingerprint(key)
		case hexSha1FingerprintLength:
			fingerprint = hexSha1Fingerprint(key)
		case md5FingerprintLength:
			fingerprint = md5Fingerprint(key)
		case 0:
			fingerprint = md5Fingerprint(key)
			return fmt.Errorf("Unable to verify identity of host.\n\nThe fingerprint of the received key was %q.", fingerprint)
		default:
			return errors.New("Unsupported host key fingerprint format")
		}

		if fingerprint != expectedFingerprint {
			return fmt.Errorf("Host key verification failed.\n\nThe fingerprint of the received key was %q.", fingerprint)
		}
		return nil
	}
}

// PublicKeys implements AuthMethod by using the given key pairs.
type PublicKeys struct {
	User   string
	Signer ssh.Signer
}

func NewPublicKeys(pemBytes []byte, passphrase string) (*PublicKeys, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(pemBytes)
	}
	if err != nil {
		return nil, err
	}

	return &PublicKeys{Signer: signer}, nil
}

func NewPublicKeysFromFile(pemFile, passphrase string) (*PublicKeys, error) {
	bytes, err := ioutil.ReadFile(pemFile)
	if err != nil {
		return nil, err
	}

	return NewPublicKeys(bytes, passphrase)
}

func MakeSessionNoTerminal(client *ssh.Client, opts ...SSHSessionOptions) (*ssh.Session, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("Failed to create session: %s", err)
	}
	for _, opt := range opts {
		err = opt(sess)
		if err != nil {
			return nil, fmt.Errorf("Failed to set session option: %s", err)
		}
	}
	// ensure terminal without color without making it interactive
	err = sess.RequestPty("vt100", 0, 0, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to request pty: %s", err)
	}
	return sess, nil
}

package sshbox

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
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

	host, port, err := net.SplitHostPort(conf.SSHUri)
	if err != nil {
		host = conf.SSHUri
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

func NewPublicKeys(pemBytes []byte, password string) (*PublicKeys, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM data")
	}
	if x509.IsEncryptedPEMBlock(block) {
		key, err := x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return nil, err
		}

		block = &pem.Block{Type: block.Type, Bytes: key}
		pemBytes = pem.EncodeToMemory(block)
	}

	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, err
	}

	return &PublicKeys{Signer: signer}, nil
}

func NewPublicKeysFromFile(pemFile, password string) (*PublicKeys, error) {
	bytes, err := ioutil.ReadFile(pemFile)
	if err != nil {
		return nil, err
	}

	return NewPublicKeys(bytes, password)
}

package sshbox

import (
	"github.com/ArthurHlt/go-socks5"
)

func OptSSHClientFactory(factory SshClientFactory) func(box *SSHBox) error {
	return func(box *SSHBox) error {
		box.sshFactory = factory
		return nil
	}
}

func OptSocksConf(conf *socks5.Config) func(box *SSHBox) error {
	return func(box *SSHBox) error {
		conf.Dial = box.socksConf.Dial
		box.socksConf = conf
		return nil
	}
}

func OptNameResolverFactory(nameResolverFactory NameResolverFactory) func(box *SSHBox) error {
	return func(box *SSHBox) error {
		box.nameResolverFactory = nameResolverFactory
		return nil
	}
}

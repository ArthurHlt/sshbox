package sshbox

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"github.com/ArthurHlt/sshbox/freeports"
	netctx "golang.org/x/net/context"
)

type NameResolverFactory func(sshBox *SSHBox) (NameResolver, error)

type NameResolver interface {
	Resolve(ctx netctx.Context, name string) (context.Context, net.IP, error)
}

type nameResolverSimple struct {
	nr *net.Resolver
}

func NewNameResolverSimple(servers []string) *nameResolverSimple {
	if len(servers) == 0 {
		return nil
	}
	return &nameResolverSimple{
		nr: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Millisecond * 100,
				}
				var conn net.Conn
				var err error
				for _, server := range servers {
					conn, err = d.DialContext(ctx, "tcp", server)
					if err == nil {
						return conn, nil
					}
				}
				return nil, err
			},
		},
	}
}

func (n nameResolverSimple) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	ips, err := n.nr.LookupIPAddr(ctx, name)
	if err != nil {
		return ctx, net.IP{}, err
	}
	if len(ips) == 0 {
		return ctx, net.IP{}, nil
	}
	for _, i := range ips {
		if i.IP.To4() != nil {
			return ctx, i.IP, nil
		}
	}
	return ctx, net.IP{}, nil
}

type sshDnsResolver struct {
	dnsConfig *DnsConfig
}

func DnsConfFromSSH(sshBox *SSHBox) (*DnsConfig, error) {
	session, err := sshBox.SSHClient().NewSession()
	if err != nil {
		return nil, err
	}
	b, err := session.Output("cat /etc/resolv.conf")
	session.Close() // close even on success
	if err != nil {
		return nil, err
	}
	dnsConf := dnsReadConfig(bytes.NewReader(b))
	return dnsConf, nil
}

func NameResolverFactorySSH(sshBox *SSHBox) (NameResolver, error) {
	dnsConf, err := DnsConfFromSSH(sshBox)
	if err != nil {
		return nil, err
	}
	tunnels, err := DNSServerToTunnel(dnsConf.Servers)
	if err != nil {
		return nil, err
	}
	if len(tunnels) == 0 {
		return nil, nil
	}

	servers := make([]string, len(tunnels))
	for i, tunnel := range tunnels {
		startListen := make(chan bool, 1)
		errTunnel := make(chan error, 1)
		go func() {
			err := sshBox.listenLocal(nil, tunnel, startListen)
			if err != nil {
				errTunnel <- err
			}
		}()
		servers[i] = fmt.Sprintf("127.0.0.1:%d", tunnel.LocalPort)
		select {
		case err := <-errTunnel:
			return nil, err
		case <-startListen:
			continue
		}
	}

	return NewNameResolverSimple(servers), nil
}

func DNSServerToTunnel(dnsservers []string) ([]*TunnelTarget, error) {
	if len(dnsservers) == 0 {
		return []*TunnelTarget{}, nil
	}
	targets := make([]*TunnelTarget, len(dnsservers))
	for i, server := range dnsservers {
		port, err := freeports.FreePort()
		if err != nil {
			return targets, err
		}
		targets[i] = &TunnelTarget{
			RemoteHost: server,
			RemotePort: 53,
			LocalPort:  port,
			Reverse:    false,
		}
		err = targets[i].CheckAndFill()
		if err != nil {
			return []*TunnelTarget{}, err
		}
	}
	return targets, nil
}

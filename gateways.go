package sshbox

import (
	"fmt"
	"net"
	"strconv"

	"github.com/phayes/freeport"
)

type GatewayInfo struct {
	SrcSSHUri  string
	LocalPort  int
	RemoteHost string
	RemotePort int
}

type Gateways struct {
	gateways []*SSHConf
	gwBoxes  []*SSHBox
}

func NewGateways(gateways []*SSHConf) *Gateways {
	return &Gateways{gateways: gateways, gwBoxes: make([]*SSHBox, 0)}
}

func (g *Gateways) RunGateways(toHost string) (string, error) {
	if len(g.gateways) == 0 {
		return toHost, nil
	}
	sshUris := make([]GatewayInfo, len(g.gateways))
	for i, gateway := range g.gateways {
		gi := GatewayInfo{}
		port, err := freeport.GetFreePort()
		if err != nil {
			return "", err
		}
		gi.LocalPort = port
		if i == 0 {
			gi.SrcSSHUri = gateway.Host
		} else {
			gi.SrcSSHUri = fmt.Sprintf("127.0.0.1:%d", sshUris[i-1].LocalPort)
		}
		if len(g.gateways) == i+1 {
			sshUris[i] = gi
			continue
		}
		remoteHost, remotePortRaw, err := net.SplitHostPort(g.gateways[i+1].Host)
		if err != nil {
			return "", err
		}
		gi.RemoteHost = remoteHost
		gi.RemotePort, _ = strconv.Atoi(remotePortRaw)
		sshUris[i] = gi
	}

	for i, gateway := range g.gateways {
		sb, err := NewSSHBox(SSHConf{
			Host:               sshUris[i].SrcSSHUri,
			User:               gateway.User,
			Password:           gateway.Password,
			PrivateKey:         gateway.PrivateKey,
			Passphrase:         gateway.Passphrase,
			HostKeyFingerprint: gateway.HostKeyFingerprint,
			SSHAuthSock:        gateway.SSHAuthSock,
		})
		if err != nil {
			return "", err
		}
		sub := sb.emitter.OnStartTunnels()
		var target *TunnelTarget
		if i == len(g.gateways)-1 {
			remoteHost, remotePortRaw, err := net.SplitHostPort(toHost)
			if err != nil {
				return "", err
			}
			remotePort, _ := strconv.Atoi(remotePortRaw)
			target = &TunnelTarget{
				RemoteHost: remoteHost,
				RemotePort: remotePort,
				LocalPort:  sshUris[i].LocalPort,
			}
		} else {
			target = &TunnelTarget{
				RemoteHost: sshUris[i].RemoteHost,
				RemotePort: sshUris[i].RemotePort,
				LocalPort:  sshUris[i].LocalPort,
			}
		}
		err = target.CheckAndFill()
		if err != nil {
			return "", err
		}
		go func() {
			err := sb.StartTunnels([]*TunnelTarget{target})
			if err != nil {
				logger.Errorf("Could not start tunnel for gateways: %s", err.Error())
			}
		}()
		<-sub
		g.gwBoxes = append(g.gwBoxes, sb)
	}
	return fmt.Sprintf("127.0.0.1:%d", sshUris[len(sshUris)-1].LocalPort), nil
}

func (g *Gateways) Close() {
	for i := len(g.gwBoxes) - 1; i >= 0; i-- {
		g.gwBoxes[i].Close()
	}
}

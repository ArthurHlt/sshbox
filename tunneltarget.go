package sshbox

import (
	"fmt"
	"github.com/ArthurHlt/sshbox/freeports"
)

type TunnelTargets []*TunnelTarget

type TunnelTarget struct {
	Network    string
	RemoteHost string
	RemotePort int
	LocalPort  int
	Reverse    bool
}

func (c *TunnelTarget) CheckAndFill() error {
	var err error
	if c.LocalPort <= 0 {
		c.LocalPort, err = freeports.FreePort()
		if err != nil {
			return err
		}
	} else {
		freeports.RegisterPort(c.LocalPort)
	}
	if c.Network == "" {
		c.Network = "tcp"
	}
	if c.Reverse {
		c.RemoteHost = "127.0.0.1"
		if c.RemotePort <= 0 {
			c.RemotePort = c.LocalPort
		}
	}
	return nil
}

func (c TunnelTarget) String() string {
	if !c.Reverse {
		return fmt.Sprintf("%s://localhost:%d -> %s://%s:%d", c.Network, c.LocalPort, c.Network, c.RemoteHost, c.RemotePort)
	}
	return fmt.Sprintf("%s://%s:%d -> %s://localhost:%d", c.Network, c.RemoteHost, c.RemotePort, c.Network, c.LocalPort)
}

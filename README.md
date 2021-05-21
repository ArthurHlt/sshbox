# SSHBox

This is a library for ease of native ssh with https://pkg.go.dev/golang.org/x/crypto/ssh . This help you to:

- Create tunnels on ssh server
- Create reverse tunnels on ssh server
- Create socks5 server on ssh server, you can also have dns resolution from nameserver on ssh server which let you set `socks5h` server
- Gateway(s) creation for accessing ssh server in chainable way
- Have an interactive shell on ssh server 

**Note**: Use https://pkg.go.dev/golang.org/x/crypto/ssh make the library totally standalone from `ssh` command line from a linux server. 
This liberate you from having putty on windows for example.

## Usage 

```go
package main

import (
	"github.com/ArthurHlt/sshbox"
)

func main() {
	sb, err := sshbox.NewSSHBox(sshbox.SSHConf{
		Host:      "url.com",
		User:       "root",
		Password:   "a password",
		NoSSHAgent: true,
	})
	if err != nil {
		panic(err)
	}

	// create tunnels
	// this will let you call access to something running on port 8080 in your ssh server on port 8080 on localhost
	// if reverse is true, this is inverted, ssh server will access to something running locally on port 8080
	go sb.StartTunnels([]*sshbox.TunnelTarget{
		{
			Network:    "tcp",
			RemoteHost: "127.0.0.1",
			RemotePort: 8080,
			LocalPort:  8080,
			Reverse:    false,
		},
	})
	// Create a socks5 server on udp and tcp
	// you can now use with env var https_proxy=socks5h://localhost:9090 and http_proxy=socks5h://localhost:9090
	go sb.StartSocksServer(9090, "tcp")
	go sb.StartSocksServer(9090, "udp")
	// This will open a shell on ssh server
	interact := sshbox.NewInteractiveSSH(sb)
	panic(interact.Interactive())
	// panic(sb.StartSocksServer(9090, "tcp"))
}
```
package sshbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ArthurHlt/go-socks5"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type SSHBoxOptions func(sshBox *SSHBox) error

type SSHBox struct {
	config              SSHConf
	sshClient           *ssh.Client
	keepaliveStopCh     chan struct{}
	sshFactory          SshClientFactory
	socksConf           *socks5.Config
	nameResolverFactory NameResolverFactory
	cachedNameResolver  NameResolver
	emitter             *Emitter
}

func NewSSHBox(config SSHConf, opts ...SSHBoxOptions) (*SSHBox, error) {
	t := &SSHBox{
		config:              config,
		keepaliveStopCh:     make(chan struct{}),
		sshFactory:          DefaultSshClientFactory,
		nameResolverFactory: NameResolverFactorySSH,
		emitter:             NewEmitter(),
	}
	var err error
	t.sshClient, err = t.makeSSHClient(t.keepaliveStopCh)
	if err != nil {
		return nil, err
	}
	t.socksConf = &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return t.sshClient.Dial(network, addr)
		},
	}
	for _, opt := range opts {
		err := opt(t)
		if err != nil {
			return nil, err
		}
	}
	return t, err
}

func (t *SSHBox) listenLocal(wg *sync.WaitGroup, target *TunnelTarget, startListen chan bool) error {
	if wg != nil {
		defer wg.Done()
	}
	listener, err := net.Listen(target.Network, fmt.Sprintf("127.0.0.1:%d", target.LocalPort))
	if err != nil {
		return errLoadErrorf("error on listening: %s", err.Error())
	}

	defer listener.Close()
	go func() {
		select {
		case <-t.emitter.OnStopTunnels():
			log.Debug("Stopping tunnels cause of emitted stop tunnels message")
			listener.Close()
			return
		}
	}()
	startListen <- true
	for {
		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return errLoadErrorf("error on accept: %s", err.Error())
		}

		go HandleTunnelClient(conn, t.sshClient, target)
	}
	return nil
}

func (t *SSHBox) listenRLocal(wg *sync.WaitGroup, target *TunnelTarget, startListen chan bool) error {
	if wg != nil {
		defer wg.Done()
	}
	targetAddr := fmt.Sprintf("%s:%d", target.RemoteHost, target.RemotePort)
	listener, err := t.sshClient.Listen(target.Network, targetAddr)
	if err != nil {
		log.Fatalln(fmt.Printf("Listen open port ON remote server error: %s", err))
	}
	defer listener.Close()
	go func() {
		select {
		case <-t.emitter.OnStopTunnels():
			log.Debug("Stopping reverse tunnels cause of emitted stop tunnels message")
			listener.Close()
			return
		}
	}()
	startListen <- true
	for {
		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return errLoadErrorf("error on accept: %s", err.Error())
		}

		go HandleRTunnelClient(conn, target)

	}

}

func (t *SSHBox) makeSSHClient(keepaliveStopCh chan struct{}) (*ssh.Client, error) {

	serverConn, err := t.sshFactory(t.config)
	if err != nil {
		return nil, err
	}
	go t.keepalive(serverConn, keepaliveStopCh)
	go func() {
		select {
		case <-t.emitter.OnStopSsh():
			log.Debug("Stopping ssh client cause of emitted stop ssh message")
			t.emitter.EmitStopSocks()
			t.emitter.EmitStopTunnels()
			serverConn.Close()
			return
		}
	}()
	return serverConn, nil
}

func (t *SSHBox) SSHClient() *ssh.Client {
	return t.sshClient
}

func (t *SSHBox) StartTunnels(tunnelTargets []*TunnelTarget) error {
	wg := &sync.WaitGroup{}
	wg.Add(len(tunnelTargets))
	for _, target := range tunnelTargets {
		startListen := make(chan bool, 1)
		errTunnel := make(chan error, 1)
		if !target.Reverse {
			go func() {
				err := t.listenLocal(wg, target, startListen)
				if err != nil {
					errTunnel <- err
				}
			}()
		} else {
			go func() {
				err := t.listenRLocal(wg, target, startListen)
				if err != nil {
					errTunnel <- err
				}
			}()
		}
		select {
		case err := <-errTunnel:
			return err
		case <-startListen:
			continue
		}

	}
	t.emitter.emitStartTunnels()
	wg.Wait()
	return nil
}

func (t *SSHBox) nameResolver() (NameResolver, error) {
	if t.cachedNameResolver != nil {
		return t.cachedNameResolver, nil
	}
	nameResolver, err := t.nameResolverFactory(t)
	if err != nil {
		return nil, err
	}
	t.cachedNameResolver = nameResolver
	return t.cachedNameResolver, nil
}

func (t *SSHBox) StartSocksServer(port int, network string) error {
	if network == "" {
		network = "tcp"
	}

	nameResolver, err := t.nameResolver()
	if err != nil {
		return err
	}

	t.socksConf.Resolver = nameResolver
	server, err := socks5.New(t.socksConf)
	if err != nil {
		return errLoadErrorf("new socks5 server: %s", err) // not tested
	}

	listener, err := net.Listen(network, fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-t.emitter.OnStopSocks():
			log.Debug("Stopping socks cause of emitted stop socks message")
			listener.Close()
			return
		}
	}()
	return server.Serve(listener)
}

func (t *SSHBox) StopSocksServer() {
	t.emitter.EmitStopSocks()
}

func (t *SSHBox) StopTunnelsServer() {
	t.emitter.EmitStopTunnels()
}

func (t *SSHBox) StopSSH() {
	t.emitter.EmitStopSsh()
}

func HandleTunnelClient(client net.Conn, sshClient *ssh.Client, target *TunnelTarget) {
	defer client.Close()
	targetAddr := fmt.Sprintf("%s:%d", target.RemoteHost, target.RemotePort)
	remoteConn, err := sshClient.Dial(target.Network, targetAddr)
	if err != nil {
		fmt.Printf("connect to %s failed: %s\n", targetAddr, err.Error())
		return
	}
	defer remoteConn.Close()

	copyData(client, remoteConn)
}

func HandleRTunnelClient(client net.Conn, target *TunnelTarget) {
	defer client.Close()
	localAddr := fmt.Sprintf("127.0.0.1:%d", target.LocalPort)
	local, err := net.Dial(target.Network, localAddr)
	if err != nil {
		fmt.Printf("connect to local %s failed: %s\n", localAddr, err.Error())
		return
	}
	defer local.Close()

	copyData(client, local)
}

func copyData(client, server net.Conn) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	// Start remote -> local data transfer
	go func() {
		_, err := io.Copy(client, server)
		if err != nil {
			log.Debugf("error while copy remote->local: ", err)
		}
		wg.Done()
	}()

	// Start local -> remote data transfer
	go func() {
		_, err := io.Copy(server, client)
		if err != nil {
			log.Debugf("error while copy local->remote: ", err)
		}
		wg.Done()
	}()

	wg.Wait()
}

func (t SSHBox) Emitter() *Emitter {
	return t.emitter
}

func (t *SSHBox) keepalive(conn ssh.Conn, stopCh chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			_, _, err := conn.SendRequest("keepalive@sshbox.com", true, nil)
			if err != nil {
				log.Warningf("Stopping socks and tunnels because ssh interrupted: %s", err.Error())
				t.emitter.EmitStopSocks()
				t.emitter.EmitStopTunnels()
				return
			}
		case <-stopCh:
			ticker.Stop()
			return
		}
	}
}
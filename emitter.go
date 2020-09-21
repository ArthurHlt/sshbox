package sshbox

import (
	"fmt"

	"github.com/olebedev/emitter"
)

type Emitter struct {
	e *emitter.Emitter
}

func NewEmitter() *Emitter {
	return &Emitter{
		e: emitter.New(uint(100)),
	}
}

func (em *Emitter) EmitStopSsh() {
	em.e.Emit("sshbox_stop_ssh", fmt.Errorf(""))
}

func (em *Emitter) OnStopSsh() <-chan emitter.Event {
	return em.e.On("sshbox_stop_ssh", emitter.Sync)
}

func (em *Emitter) OffStopSsh(events ...<-chan emitter.Event) {
	em.e.Off("sshbox_stop_ssh", events...)
}

func (em *Emitter) ListenersStopSsh() []<-chan emitter.Event {
	return em.e.Listeners("sshbox_stop_ssh")
}

func (em *Emitter) EmitStopTunnels() {
	em.e.Emit("sshbox_stop_tunnels", fmt.Errorf(""))
}

func (em *Emitter) OnStopTunnels() <-chan emitter.Event {
	return em.e.On("sshbox_stop_tunnels", emitter.Sync)
}

func (em *Emitter) OffStopTunnels(events ...<-chan emitter.Event) {
	em.e.Off("sshbox_stop_tunnels", events...)
}

func (em *Emitter) ListenersStopTunnels() []<-chan emitter.Event {
	return em.e.Listeners("sshbox_stop_tunnels")
}

func (em *Emitter) EmitStopSocks() {
	em.e.Emit("sshbox_stop_socks", fmt.Errorf(""))
}

func (em *Emitter) OnStopSocks() <-chan emitter.Event {
	return em.e.On("sshbox_stop_socks", emitter.Sync)
}

func (em *Emitter) OffStopSocks(events ...<-chan emitter.Event) {
	em.e.Off("sshbox_stop_socks", events...)
}

func (em *Emitter) ListenersStopSocks() []<-chan emitter.Event {
	return em.e.Listeners("sshbox_stop_socks")
}

func (em *Emitter) emitStartTunnels() {
	em.e.Emit("sshbox_start_tunnels", fmt.Errorf(""))
}

func (em *Emitter) OnStartTunnels() <-chan emitter.Event {
	return em.e.On("sshbox_start_tunnels", emitter.Sync)
}

func (em *Emitter) OffStartTunnels(events ...<-chan emitter.Event) {
	em.e.Off("sshbox_start_tunnels", events...)
}

func (em *Emitter) ListenersStartTunnels() []<-chan emitter.Event {
	return em.e.Listeners("sshbox_start_tunnels")
}

func (em *Emitter) ToError(evt emitter.Event) error {
	if len(evt.Args) == 0 {
		return nil
	}
	err, ok := evt.Args[0].(error)
	if !ok {
		return nil
	}
	return err
}

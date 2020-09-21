package freeports

import (
	"github.com/phayes/freeport"
)

var portsTaken = make(map[int]bool)

func RegisterPort(port int) {
	portsTaken[port] = true
}

func FreePort() (int, error) {
	port, err := freeport.GetFreePort()
	if err != nil {
		return 0, err
	}
	if _, ok := portsTaken[port]; !ok {
		RegisterPort(port)
		return port, nil
	}
	return FreePort()
}

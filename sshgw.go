package sshbox

import "fmt"

type SShInGateways struct {
	gateways   *Gateways
	currentBox *SSHBox
}

func NewSShInGateways(sshConf *SSHConf, gatewaysConf []*SSHConf) (*SShInGateways, error) {
	var gateways *Gateways
	if len(gatewaysConf) > 0 {
		gateways = NewGateways(gatewaysConf)
		sshUri, err := gateways.RunGateways(sshConf.Host)
		if err != nil {
			return nil, fmt.Errorf("failed to run gateways: %s", err)
		}
		sshConf.Host = sshUri
	}
	sb, err := NewSSHBox(*sshConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create sshbox: %s", err)
	}
	return &SShInGateways{
		gateways:   gateways,
		currentBox: sb,
	}, nil
}

func (S *SShInGateways) SSHBox() *SSHBox {
	return S.currentBox
}

func (S *SShInGateways) Close() {
	if S.gateways != nil {
		S.gateways.Close()
	}
	S.currentBox.Close()
}

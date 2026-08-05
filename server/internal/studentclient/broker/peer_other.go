//go:build !linux

package broker

import (
	"fmt"
	"net"
)

// PeerCred is the authenticated peer identity (stub on non-Linux).
type PeerCred struct {
	PID int32
	UID uint32
	GID uint32
}

func peerCredFromConn(c net.Conn) (*PeerCred, error) {
	return nil, fmt.Errorf("peercred: SO_PEERCRED only supported on linux")
}

func chownSocketGroup(path, groupName string) error {
	return nil
}

func groupHasUID(groupName string, uid uint32) bool {
	return false
}

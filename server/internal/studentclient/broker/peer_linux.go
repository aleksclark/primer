//go:build linux

package broker

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

// PeerCred is the authenticated peer identity from SO_PEERCRED.
type PeerCred struct {
	PID int32
	UID uint32
	GID uint32
}

// peerCredFromConn returns Linux SO_PEERCRED for a Unix conn.
func peerCredFromConn(c net.Conn) (*PeerCred, error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("peercred: not a unix connection")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("peercred: syscall conn: %w", err)
	}
	var (
		cred *unix.Ucred
		opErr error
	)
	err = raw.Control(func(fd uintptr) {
		cred, opErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return nil, fmt.Errorf("peercred: control: %w", err)
	}
	if opErr != nil {
		return nil, fmt.Errorf("peercred: getsockopt: %w", opErr)
	}
	if cred == nil {
		return nil, fmt.Errorf("peercred: nil ucred")
	}
	return &PeerCred{PID: cred.Pid, UID: cred.Uid, GID: cred.Gid}, nil
}

// chownSocketGroup sets the socket's group ownership (keeps current uid).
func chownSocketGroup(path, groupName string) error {
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, -1, gid)
}

// groupHasUID reports whether uid is a member of groupName (by name).
func groupHasUID(groupName string, uid uint32) bool {
	g, err := user.LookupGroup(groupName)
	if err != nil {
		return false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return false
	}
	// Primary group match via /etc/passwd.
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err == nil {
		if u.Gid == g.Gid {
			return true
		}
		// Supplementary groups.
		ids, err := u.GroupIds()
		if err == nil {
			for _, id := range ids {
				if id == g.Gid || id == strconv.Itoa(gid) {
					return true
				}
			}
		}
	}
	return false
}

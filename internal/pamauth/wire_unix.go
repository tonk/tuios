//go:build !windows

package pamauth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
)

func writeMessage(conn *net.UnixConn, msgType byte, payload []byte, fd int) error {
	if len(payload) > maxPayload {
		return fmt.Errorf("payload too large: %d bytes", len(payload))
	}
	buf := make([]byte, 0, headerLen+len(payload))
	buf = append(buf, msgType)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(payload)))
	buf = append(buf, n[:]...)
	buf = append(buf, payload...)

	var oob []byte
	if fd >= 0 {
		oob = syscall.UnixRights(fd)
	}
	_, _, err := conn.WriteMsgUnix(buf, oob, nil)
	return err
}

func readMessage(conn *net.UnixConn) (msgType byte, payload []byte, fd int, err error) {
	buf := make([]byte, maxRead)
	oob := make([]byte, syscall.CmsgSpace(4))
	n, oobn, flags, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return 0, nil, -1, err
	}
	if flags&syscall.MSG_CTRUNC != 0 {
		return 0, nil, -1, errors.New("ancillary data truncated (MSG_CTRUNC)")
	}
	if n < headerLen {
		return 0, nil, -1, fmt.Errorf("short message: %d bytes", n)
	}
	msgType = buf[0]
	declaredLen := int(binary.BigEndian.Uint32(buf[1:headerLen]))
	if headerLen+declaredLen != n {
		return 0, nil, -1, fmt.Errorf("message length mismatch: header says %d, read %d", declaredLen, n-headerLen)
	}
	payload = buf[headerLen:n]

	fd = -1
	if oobn > 0 {
		scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return 0, nil, -1, fmt.Errorf("parsing control message: %w", err)
		}
		if len(scms) == 1 {
			if fds, err := syscall.ParseUnixRights(&scms[0]); err == nil && len(fds) == 1 {
				fd = fds[0]
			}
		}
	}
	return msgType, payload, fd, nil
}

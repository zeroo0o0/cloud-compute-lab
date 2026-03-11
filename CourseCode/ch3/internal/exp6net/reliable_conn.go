package exp6net

import (
	"net"
	"time"
	"warzone/exp6/internal/exp6proto"
)

// ReliableConn wraps framing + JSON + timeout receive.
type ReliableConn struct {
	conn net.Conn
}

func NewReliableConn(conn net.Conn) *ReliableConn {
	return &ReliableConn{conn: conn}
}

func (rc *ReliableConn) Send(v any) error {
	return exp6proto.SendJSON(rc.conn, v)
}

// Recv blocks until data, timeout, or error.
func (rc *ReliableConn) Recv(timeout time.Duration, out any) error {
	if err := rc.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	return exp6proto.RecvJSON(rc.conn, out)
}

func (rc *ReliableConn) Close() error {
	return rc.conn.Close()
}

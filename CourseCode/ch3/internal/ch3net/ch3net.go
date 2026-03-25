package ch3net

import (
	"ch3/internal/ch3proto"
	"net"
	"sync"
	"time"
)

type ReliableConn struct {
	conn    net.Conn
	writeMu sync.Mutex
}

func NewReliableConn(conn net.Conn) *ReliableConn {
	return &ReliableConn{conn: conn}
}

func (rc *ReliableConn) Send(v any) error {
	return rc.SendTimeout(0, v)
}

func (rc *ReliableConn) SendTimeout(timeout time.Duration, v any) error {
	rc.writeMu.Lock()
	defer rc.writeMu.Unlock()

	if timeout > 0 {
		if err := rc.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		defer rc.conn.SetWriteDeadline(time.Time{})
	}
	return ch3proto.SendJSON(rc.conn, v)
}

func (rc *ReliableConn) Recv(timeout time.Duration, out any) error {
	if err := rc.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	return ch3proto.RecvJSON(rc.conn, out)
}

func (rc *ReliableConn) RawConn() net.Conn {
	return rc.conn
}

func (rc *ReliableConn) Close() error {
	return rc.conn.Close()
}

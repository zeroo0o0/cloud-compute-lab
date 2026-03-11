package ch3net

import (
	"net"
	"time"
	"warzone/ch3/internal/ch3proto"
)

type ReliableConn struct {
	conn net.Conn
}

func NewReliableConn(conn net.Conn) *ReliableConn {
	return &ReliableConn{conn: conn}
}

func (rc *ReliableConn) Send(v any) error {
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

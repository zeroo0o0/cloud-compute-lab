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

	/*
		================ 【学生重点 第三章：写入互斥与超时】 ================
		多个 goroutine 同时向同一条 TCP 连接写 JSON 时，字节可能交错。
		writeMu 保证“长度头 + JSON 正文”作为一整条消息连续写出。
		设置写 deadline 后，网络卡住时不会让发送方永久阻塞。
		================================================================
	*/
	if timeout > 0 {
		if err := rc.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		defer rc.conn.SetWriteDeadline(time.Time{})
	}
	return ch3proto.SendJSON(rc.conn, v)
}

func (rc *ReliableConn) Recv(timeout time.Duration, out any) error {
	/*
		================ 【学生重点 第三章：超时读】 ================
		僵尸连接最危险的地方是 Read 永远等不到数据，拖住主循环。
		SetReadDeadline 把“永久等待”变成“超时返回”，上层就可以把本帧
		当作 idle 处理，让房间继续推进。
		========================================================
	*/
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

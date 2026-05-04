package ch3proto

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

type FrameMessage struct {
	From string `json:"from"`
	Text string `json:"text"`
}

type InputMsg struct {
	PlayerID int    `json:"player_id"`
	Frame    int    `json:"frame"`
	Action   string `json:"action"`
}

type JoinMsg struct {
	PlayerID int `json:"player_id"`
}

type PlayerState struct {
	ID     int  `json:"id"`
	X      int  `json:"x"`
	Y      int  `json:"y"`
	HP     int  `json:"hp"`
	Online bool `json:"online"`
}

type WorldState struct {
	Frame   int           `json:"frame"`
	Players []PlayerState `json:"players"`
	Event   string        `json:"event"`
}

func SendJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > int(^uint32(0)) {
		return fmt.Errorf("payload too large: %d", len(b))
	}
	/*
		================ 【学生重点 第三章：应用层消息边界】 ================
		TCP 只保证字节按顺序到达，不保证一次 Write 对应一次 Read。
		这里先写 4 字节长度，再写 JSON 正文，相当于给每条消息加“外包装”。
		接收端只要先读长度，再按长度读满正文，就能避开粘包和半包。
		================================================================
	*/
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(b)))
	if err := writeFull(w, lenBuf[:]); err != nil {
		return err
	}
	return writeFull(w, b)
}

func RecvJSON(r io.Reader, out any) error {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return err
	}
	/*
		================ 【学生重点 第三章：按长度拆包】 ================
		io.ReadFull 会一直读到 n 个字节，或者返回错误。
		这比直接 conn.Read(buffer) 更适合做协议解析，因为它把“读到多少”
		从操作系统缓冲区的随机时机，改成了应用层协议约定的消息长度。
		============================================================
	*/
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}

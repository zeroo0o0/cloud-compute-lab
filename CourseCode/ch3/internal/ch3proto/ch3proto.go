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
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}

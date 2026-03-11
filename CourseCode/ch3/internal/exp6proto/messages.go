package exp6proto

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Step3/6/7 common messages

type FrameMessage struct {
	From string `json:"from"`
	Text string `json:"text"`
}

type InputMsg struct {
	PlayerID int    `json:"player_id"`
	Frame    int    `json:"frame"`
	Action   string `json:"action"`
}

type PlayerState struct {
	ID     int `json:"id"`
	X      int `json:"x"`
	Y      int `json:"y"`
	HP     int `json:"hp"`
	LastFx int `json:"last_fx"`
}

type WorldState struct {
	Frame   int           `json:"frame"`
	Players []PlayerState `json:"players"`
	Event   string        `json:"event"`
}

// SendJSON writes [4-byte little-endian length][json bytes]
func SendJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > int(^uint32(0)) {
		return fmt.Errorf("payload too large: %d", len(b))
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// RecvJSON reads [4-byte little-endian length] then unmarshals JSON.
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

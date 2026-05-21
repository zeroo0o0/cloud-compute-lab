package proto

// PlayerState is the authoritative position stored by storage.
type PlayerState struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type GameResponse struct {
	OK       bool         `json:"ok"`
	Layer    string       `json:"layer"`
	Error    string       `json:"error,omitempty"`
	Result   string       `json:"result,omitempty"`
	PlayerID string       `json:"playerId,omitempty"`
	Position *PlayerState `json:"position,omitempty"`
}

type ClientMsg struct {
	Type     string `json:"type"`
	PlayerID string `json:"playerId"`
	ClientID string `json:"clientId"`
	DX       int    `json:"dx"`
	DY       int    `json:"dy"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

type MoveRequest struct {
	PlayerID string `json:"playerId"`
	DX       int    `json:"dx"`
	DY       int    `json:"dy"`
}

type GameEnvelope struct {
	OK       bool         `json:"ok"`
	Layer    string       `json:"layer"`
	Error    string       `json:"error,omitempty"`
	Position *PlayerState `json:"position,omitempty"`
}

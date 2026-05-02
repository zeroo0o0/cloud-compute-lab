package proto

// Exp1 messages.
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

// Exp2 messages.
type ProcessRequest struct {
	Action string `json:"action"` // get or move
	UserID string `json:"user_id"`
	MapID  string `json:"map_id"`
	DX     int    `json:"dx"`
	DY     int    `json:"dy"`
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type ProcessResponse struct {
	OK           bool     `json:"ok"`
	Error        string   `json:"error,omitempty"`
	UserID       string   `json:"user_id,omitempty"`
	MapID        string   `json:"map_id,omitempty"`
	GameShard    string   `json:"game_shard,omitempty"`
	StorageShard string   `json:"storage_shard,omitempty"`
	Position     Position `json:"position"`
	Traces       []string `json:"traces,omitempty"`
}

type StorageSetRequest struct {
	UserID string `json:"user_id"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

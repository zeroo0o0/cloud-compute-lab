package cloudapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"battleworld/protocol"
)

const (
	CoordinatorActionRegister  = protocol.TypeRegister
	CoordinatorActionLogin     = protocol.TypeLogin
	CoordinatorActionQuick     = protocol.TypeQuickEnter
	CoordinatorActionMove      = protocol.TypeMove
	CoordinatorActionAttack    = protocol.TypeAttack
	CoordinatorActionBoss      = protocol.TypeBossAttack
	CoordinatorActionHeal      = protocol.TypeHeal
	CoordinatorActionShop      = protocol.TypeShop
	CoordinatorActionTransfer  = protocol.TypeTransfer
	CoordinatorActionSwitchMap = protocol.TypeSwitchMap
	CoordinatorActionLogout    = protocol.TypeLogout
	CoordinatorActionSnapshot  = "snapshot"
	CoordinatorActionAdmin     = protocol.TypeAdmin
)

const (
	MapActionAddOrRestore = "add_or_restore"
	MapActionRemove       = "remove"
	MapActionMove         = "move"
	MapActionAttack       = "attack"
	MapActionHeal         = "heal"
	MapActionBuy          = "buy"
	MapActionProfile      = "profile"
	MapActionReward       = "reward"
	MapActionAdjust       = "adjust_treasures"
	MapActionSnapshot     = "snapshot"
	MapActionCounts       = "counts"
	MapActionCheckpoint   = "checkpoint"
	MapActionDrainEvents  = "drain_events"
	MapActionHealth       = "health"
)

type CoordinatorRequest struct {
	Action      string `json:"action"`
	AdminAction string `json:"admin_action,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	Confirm     string `json:"confirm,omitempty"`
	Dir         string `json:"dir,omitempty"`
	MapID       string `json:"map_id,omitempty"`
	Item        string `json:"item,omitempty"`
	Target      string `json:"target,omitempty"`
	Amount      int    `json:"amount,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
}

type CoordinatorResponse struct {
	OK    bool                 `json:"ok"`
	Error string               `json:"error,omitempty"`
	Text  string               `json:"text,omitempty"`
	State *protocol.WorldState `json:"state,omitempty"`
}

type MapRequest struct {
	Action        string                `json:"action"`
	Username      string                `json:"username,omitempty"`
	Dir           string                `json:"dir,omitempty"`
	Item          string                `json:"item,omitempty"`
	Profile       *protocol.UserProfile `json:"profile,omitempty"`
	TreasureDelta int                   `json:"treasure_delta,omitempty"`
	VictoryDelta  int                   `json:"victory_delta,omitempty"`
	Delta         int                   `json:"delta,omitempty"`
	NodeID        string                `json:"node_id,omitempty"`
}

type MapCounts struct {
	Players   int   `json:"players"`
	NPCs      int   `json:"npcs"`
	Treasures int   `json:"treasures"`
	Version   int64 `json:"version"`
}

type MapEventBundle struct {
	Events []string `json:"events"`
}

type MapResponse struct {
	OK             bool                    `json:"ok"`
	Error          string                  `json:"error,omitempty"`
	Event          string                  `json:"event,omitempty"`
	TargetUsername string                  `json:"target_username,omitempty"`
	TargetEvent    string                  `json:"target_event,omitempty"`
	Profile        *protocol.UserProfile   `json:"profile,omitempty"`
	Map            *protocol.MapView       `json:"map,omitempty"`
	Counts         *MapCounts              `json:"counts,omitempty"`
	Checkpoint     *protocol.MapCheckpoint `json:"checkpoint,omitempty"`
	Node           *protocol.NodeView      `json:"node,omitempty"`
	Bundle         *MapEventBundle         `json:"bundle,omitempty"`
}

func PostJSON[TReq any, TResp any](client *http.Client, url string, req TReq, resp *TResp) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpResp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode >= 300 {
		return fmt.Errorf("请求 %s 失败：%s", url, httpResp.Status)
	}
	if err := json.NewDecoder(httpResp.Body).Decode(resp); err != nil {
		return err
	}
	return nil
}

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func NormalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return strings.TrimRight(raw, "/")
	}
	return "http://" + strings.TrimRight(raw, "/")
}

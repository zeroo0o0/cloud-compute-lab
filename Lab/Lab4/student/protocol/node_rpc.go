package protocol

// NodeRequest 网关发送给游戏节点的 RPC 请求
type NodeRequest struct {
	Action   string       `json:"action"`
	MapID    string       `json:"map_id,omitempty"`
	Username string       `json:"username,omitempty"`
	Dir      string       `json:"dir,omitempty"`
	Item     string       `json:"item,omitempty"`
	Profile  *UserProfile `json:"profile,omitempty"`
	Treasure int          `json:"treasure,omitempty"`
	Victory  int          `json:"victory,omitempty"`
}

// NodeResponse 游戏节点返回给网关的 RPC 响应
type NodeResponse struct {
	OK         bool           `json:"ok"`
	Error      string         `json:"error,omitempty"`
	Event      string         `json:"event,omitempty"`
	Event2     string         `json:"event2,omitempty"`
	Target     string         `json:"target,omitempty"`
	Profile    *UserProfile   `json:"profile,omitempty"`
	State      *MapView       `json:"state,omitempty"`
	Checkpoint *MapCheckpoint `json:"checkpoint,omitempty"`
	Players    int            `json:"players,omitempty"`
	NPCs       int            `json:"npcs,omitempty"`
	Treasures  int            `json:"treasures,omitempty"`
	Version    int64          `json:"version,omitempty"`
}

// 节点 RPC 动作常量
const (
	NodeActionPing         = "ping"
	NodeActionAddPlayer    = "add_player"
	NodeActionRemovePlayer = "remove_player"
	NodeActionMovePlayer   = "move_player"
	NodeActionAttack       = "attack"
	NodeActionHeal         = "heal"
	NodeActionBuyItem      = "buy_item"
	NodeActionBossAttack   = "boss_attack"
	NodeActionSnapshot     = "snapshot"
	NodeActionCounts       = "counts"
	NodeActionCheckpoint   = "checkpoint"
	NodeActionReward       = "reward"
	NodeActionProfile      = "profile"
)

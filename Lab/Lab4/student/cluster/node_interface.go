package cluster

import (
	"battleworld/protocol"
	"battleworld/world"
)

// NodeServiceInterface 节点服务接口，本地和远程节点都实现此接口
type NodeServiceInterface interface {
	ID() string
	Start() error
	Stop() error
	IsHealthy() bool
	SetHealthy(healthy bool) bool
	Ping() bool

	InstallPrimaryMap(cfg world.MapConfig)
	RestorePrimaryMap(cfg world.MapConfig, cp protocol.MapCheckpoint)
	RemoveHostedMap(mapID string)

	AddPlayer(mapID string, profile *protocol.UserProfile)
	RemovePlayer(mapID, username string) (protocol.UserProfile, bool)
	MovePlayer(mapID, username, dir string) (string, protocol.UserProfile, bool)
	Attack(mapID, username string) (string, string, string, protocol.UserProfile, bool)
	Heal(mapID, username string) (string, protocol.UserProfile, bool)
	BuyItem(mapID, username, item string) (string, protocol.UserProfile, bool)
	Profile(mapID, username string) (protocol.UserProfile, bool)
	RewardPlayer(mapID, username string, treasureDelta, victoryDelta int) (protocol.UserProfile, bool)

	Snapshot(mapID string) (protocol.MapView, error)
	Counts(mapID string) (int, int, int, int64, error)
	Checkpoint(mapID string) (protocol.MapCheckpoint, error)
	BackgroundStep() []MapEvents

	StoreReplica(cp protocol.MapCheckpoint)
	Promote(mapID string, cfg world.MapConfig) error
	View() protocol.NodeView
}

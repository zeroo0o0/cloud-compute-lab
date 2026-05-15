package cluster

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"battleworld/protocol"
	"battleworld/world"
)

// RemoteNodeService 远程节点 RPC 客户端（微服务模式）
type RemoteNodeService struct {
	mu            sync.RWMutex
	id            string
	addr          string
	healthy       bool
	lastHeartbeat time.Time
}

func newRemoteNodeService(id, addr string) *RemoteNodeService {
	return &RemoteNodeService{
		id:            id,
		addr:          addr,
		healthy:       true,
		lastHeartbeat: time.Now(),
	}
}

func (r *RemoteNodeService) ID() string { return r.id }

func (r *RemoteNodeService) Start() error {
	r.mu.Lock()
	r.healthy = true
	r.lastHeartbeat = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *RemoteNodeService) Stop() error {
	r.mu.Lock()
	r.healthy = false
	r.mu.Unlock()
	return nil
}

func (r *RemoteNodeService) IsHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.healthy
}

func (r *RemoteNodeService) SetHealthy(healthy bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	wasHealthy := r.healthy
	r.healthy = healthy
	if healthy {
		r.lastHeartbeat = time.Now()
	}
	return wasHealthy
}

func (r *RemoteNodeService) Ping() bool {
	conn, err := net.DialTimeout("tcp", r.addr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (r *RemoteNodeService) InstallPrimaryMap(cfg world.MapConfig) {
	// 远程节点通过 RPC 通知安装地图，此处简化为无操作
}

func (r *RemoteNodeService) RestorePrimaryMap(cfg world.MapConfig, cp protocol.MapCheckpoint) {
	// 远程节点通过 RPC 通知恢复地图，此处简化为无操作
}

func (r *RemoteNodeService) RemoveHostedMap(mapID string) {
	// 远程节点通过 RPC 通知移除地图，此处简化为无操作
}

func (r *RemoteNodeService) rpcCall(req protocol.NodeRequest) (protocol.NodeResponse, error) {
	conn, err := net.DialTimeout("tcp", r.addr, 3*time.Second)
	if err != nil {
		return protocol.NodeResponse{}, fmt.Errorf("连接节点 %s 失败: %w", r.id, err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(req); err != nil {
		return protocol.NodeResponse{}, fmt.Errorf("发送请求失败: %w", err)
	}

	var resp protocol.NodeResponse
	if err := decoder.Decode(&resp); err != nil {
		return protocol.NodeResponse{}, fmt.Errorf("接收响应失败: %w", err)
	}
	return resp, nil
}

func (r *RemoteNodeService) AddPlayer(mapID string, profile *protocol.UserProfile) {
	_, _ = r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionAddPlayer,
		MapID:    mapID,
		Profile:  profile,
	})
}

func (r *RemoteNodeService) RemovePlayer(mapID, username string) (protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionRemovePlayer,
		MapID:    mapID,
		Username: username,
	})
	if err != nil || !resp.OK || resp.Profile == nil {
		return protocol.UserProfile{}, false
	}
	return *resp.Profile, true
}

func (r *RemoteNodeService) MovePlayer(mapID, username, dir string) (string, protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionMovePlayer,
		MapID:    mapID,
		Username: username,
		Dir:      dir,
	})
	if err != nil || !resp.OK {
		return "", protocol.UserProfile{}, false
	}
	profile := protocol.UserProfile{}
	if resp.Profile != nil {
		profile = *resp.Profile
	}
	return resp.Event, profile, true
}

func (r *RemoteNodeService) Attack(mapID, username string) (string, string, string, protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionAttack,
		MapID:    mapID,
		Username: username,
	})
	if err != nil || !resp.OK {
		return "", "", "", protocol.UserProfile{}, false
	}
	profile := protocol.UserProfile{}
	if resp.Profile != nil {
		profile = *resp.Profile
	}
	return resp.Event, resp.Target, resp.Event2, profile, true
}

func (r *RemoteNodeService) Heal(mapID, username string) (string, protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionHeal,
		MapID:    mapID,
		Username: username,
	})
	if err != nil || !resp.OK {
		return "", protocol.UserProfile{}, false
	}
	profile := protocol.UserProfile{}
	if resp.Profile != nil {
		profile = *resp.Profile
	}
	return resp.Event, profile, true
}

func (r *RemoteNodeService) BuyItem(mapID, username, item string) (string, protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionBuyItem,
		MapID:    mapID,
		Username: username,
		Item:     item,
	})
	if err != nil || !resp.OK {
		return "", protocol.UserProfile{}, false
	}
	profile := protocol.UserProfile{}
	if resp.Profile != nil {
		profile = *resp.Profile
	}
	return resp.Event, profile, true
}

func (r *RemoteNodeService) Profile(mapID, username string) (protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionProfile,
		MapID:    mapID,
		Username: username,
	})
	if err != nil || !resp.OK || resp.Profile == nil {
		return protocol.UserProfile{}, false
	}
	return *resp.Profile, true
}

func (r *RemoteNodeService) RewardPlayer(mapID, username string, treasureDelta, victoryDelta int) (protocol.UserProfile, bool) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action:   protocol.NodeActionReward,
		MapID:    mapID,
		Username: username,
		Treasure: treasureDelta,
		Victory:  victoryDelta,
	})
	if err != nil || !resp.OK || resp.Profile == nil {
		return protocol.UserProfile{}, false
	}
	return *resp.Profile, true
}

func (r *RemoteNodeService) Snapshot(mapID string) (protocol.MapView, error) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action: protocol.NodeActionSnapshot,
		MapID:  mapID,
	})
	if err != nil {
		return protocol.MapView{}, err
	}
	if !resp.OK {
		return protocol.MapView{}, fmt.Errorf("%s", resp.Error)
	}
	if resp.State == nil {
		return protocol.MapView{}, fmt.Errorf("节点返回空快照")
	}
	return *resp.State, nil
}

func (r *RemoteNodeService) Counts(mapID string) (int, int, int, int64, error) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action: protocol.NodeActionCounts,
		MapID:  mapID,
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if !resp.OK {
		return 0, 0, 0, 0, fmt.Errorf("%s", resp.Error)
	}
	return resp.Players, resp.NPCs, resp.Treasures, resp.Version, nil
}

func (r *RemoteNodeService) Checkpoint(mapID string) (protocol.MapCheckpoint, error) {
	resp, err := r.rpcCall(protocol.NodeRequest{
		Action: protocol.NodeActionCheckpoint,
		MapID:  mapID,
	})
	if err != nil {
		return protocol.MapCheckpoint{}, err
	}
	if !resp.OK || resp.Checkpoint == nil {
		return protocol.MapCheckpoint{}, fmt.Errorf("获取检查点失败")
	}
	return *resp.Checkpoint, nil
}

func (r *RemoteNodeService) BackgroundStep() []MapEvents {
	// 远程节点自行处理 BackgroundStep，网关不需要调用
	return nil
}

func (r *RemoteNodeService) StoreReplica(cp protocol.MapCheckpoint) {
	// 远程节点自行管理副本
}

func (r *RemoteNodeService) Promote(mapID string, cfg world.MapConfig) error {
	// 远程节点通过管理命令提升
	return nil
}

func (r *RemoteNodeService) View() protocol.NodeView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return protocol.NodeView{
		ID:            r.id,
		Addr:          r.addr,
		Healthy:       r.healthy,
		PrimaryMaps:   []string{},
		ReplicaMaps:   []string{},
		LastHeartbeat: r.lastHeartbeat.Format(time.RFC3339),
	}
}

// 确保 RemoteNodeService 实现了 NodeServiceInterface
var _ NodeServiceInterface = (*RemoteNodeService)(nil)
var _ NodeServiceInterface = (*LocalNodeService)(nil)

// 辅助函数：排序字符串切片
func sortStrings(s []string) {
	sort.Strings(s)
}

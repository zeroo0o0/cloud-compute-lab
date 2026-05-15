package cluster

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"battleworld/protocol"
	"battleworld/world"
)

// LocalNodeService 本地节点服务（单进程模式）
type LocalNodeService struct {
	mu               sync.RWMutex
	id               string
	addr             string
	healthy          bool
	lastHeartbeat    time.Time
	maps             map[string]*world.World
	replicaSnapshots map[string]protocol.MapCheckpoint
	ln               net.Listener
}

func newLocalNodeService(id, addr string) *LocalNodeService {
	return &LocalNodeService{
		id:               id,
		addr:             addr,
		healthy:          true,
		lastHeartbeat:    time.Now(),
		maps:             make(map[string]*world.World),
		replicaSnapshots: make(map[string]protocol.MapCheckpoint),
	}
}

func (n *LocalNodeService) ID() string { return n.id }

func (n *LocalNodeService) Start() error {
	n.mu.RLock()
	if n.ln != nil {
		n.mu.RUnlock()
		return nil
	}
	n.mu.RUnlock()

	ln, err := net.Listen("tcp", n.addr)
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.ln = ln
	n.lastHeartbeat = time.Now()
	n.healthy = true
	n.mu.Unlock()

	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = fmt.Fprintf(c, "node=%s ts=%d\n", n.id, time.Now().UnixNano())
			}(raw)
		}
	}()
	return nil
}

func (n *LocalNodeService) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.ln == nil {
		return nil
	}
	err := n.ln.Close()
	n.ln = nil
	n.healthy = false
	return err
}

func (n *LocalNodeService) RemoveHostedMap(mapID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.maps, mapID)
}

func (n *LocalNodeService) InstallPrimaryMap(cfg world.MapConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.maps[cfg.ID]; ok {
		return
	}
	n.maps[cfg.ID] = world.NewWorld(cfg)
}

func (n *LocalNodeService) RestorePrimaryMap(cfg world.MapConfig, cp protocol.MapCheckpoint) {
	n.mu.Lock()
	defer n.mu.Unlock()

	instance, ok := n.maps[cfg.ID]
	if !ok {
		instance = world.NewWorld(cfg)
		n.maps[cfg.ID] = instance
	}
	instance.RestoreCheckpoint(cp)
}

func (n *LocalNodeService) AddPlayer(mapID string, profile *protocol.UserProfile) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return
	}
	instance.AddOrRestorePlayer(profile)
}

func (n *LocalNodeService) RemovePlayer(mapID, username string) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.RemovePlayer(username)
}

func (n *LocalNodeService) MovePlayer(mapID, username, dir string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.MovePlayer(username, dir)
}

func (n *LocalNodeService) Attack(mapID, username string) (string, string, string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", "", "", protocol.UserProfile{}, false
	}
	return instance.Attack(username)
}

func (n *LocalNodeService) Heal(mapID, username string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.HealPlayer(username)
}

func (n *LocalNodeService) BuyItem(mapID, username, item string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.BuyItem(username, item)
}

func (n *LocalNodeService) Profile(mapID, username string) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.ProfileOf(username)
}

func (n *LocalNodeService) RewardPlayer(mapID, username string, treasureDelta, victoryDelta int) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.RewardPlayer(username, treasureDelta, victoryDelta)
}

func (n *LocalNodeService) Snapshot(mapID string) (protocol.MapView, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.MapView{}, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.id)
	}
	return instance.Snapshot(n.id), nil
}

func (n *LocalNodeService) Counts(mapID string) (int, int, int, int64, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return 0, 0, 0, 0, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.id)
	}
	players, npcs, treasures, version := instance.Counts()
	return players, npcs, treasures, version, nil
}

func (n *LocalNodeService) Checkpoint(mapID string) (protocol.MapCheckpoint, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.MapCheckpoint{}, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.id)
	}
	return instance.CaptureCheckpoint(n.id), nil
}

func (n *LocalNodeService) BackgroundStep() []MapEvents {
	n.mu.RLock()
	mapIDs := make([]string, 0, len(n.maps))
	for mapID := range n.maps {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)
	instances := make([]*world.World, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		instances = append(instances, n.maps[mapID])
	}
	n.mu.RUnlock()

	results := make([]MapEvents, 0, len(instances))
	for i, instance := range instances {
		events := instance.BackgroundStep()
		if len(events) == 0 {
			continue
		}
		results = append(results, MapEvents{MapID: mapIDs[i], Events: events})
	}
	return results
}

func (n *LocalNodeService) StoreReplica(cp protocol.MapCheckpoint) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.replicaSnapshots[cp.MapID] = cp
}

func (n *LocalNodeService) Promote(mapID string, cfg world.MapConfig) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cp, ok := n.replicaSnapshots[mapID]
	if !ok {
		return fmt.Errorf("节点 %s 上没有地图 %q 的副本快照", n.id, mapID)
	}
	instance := world.NewWorld(cfg)
	instance.RestoreCheckpoint(cp)
	n.maps[mapID] = instance
	delete(n.replicaSnapshots, mapID)
	return nil
}

func (n *LocalNodeService) View() protocol.NodeView {
	n.mu.RLock()
	defer n.mu.RUnlock()

	primaryMaps := make([]string, 0, len(n.maps))
	for mapID := range n.maps {
		primaryMaps = append(primaryMaps, mapID)
	}
	replicaMaps := make([]string, 0, len(n.replicaSnapshots))
	for mapID := range n.replicaSnapshots {
		replicaMaps = append(replicaMaps, mapID)
	}
	sort.Strings(primaryMaps)
	sort.Strings(replicaMaps)

	return protocol.NodeView{
		ID:            n.id,
		Addr:          n.addr,
		Healthy:       n.healthy,
		PrimaryMaps:   primaryMaps,
		ReplicaMaps:   replicaMaps,
		LastHeartbeat: n.lastHeartbeat.Format(time.RFC3339),
	}
}

func (n *LocalNodeService) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}

func (n *LocalNodeService) SetHealthy(healthy bool) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	wasHealthy := n.healthy
	n.healthy = healthy
	if healthy {
		n.lastHeartbeat = time.Now()
	}
	return wasHealthy
}

func (n *LocalNodeService) Ping() bool {
	return ping(n.addr)
}

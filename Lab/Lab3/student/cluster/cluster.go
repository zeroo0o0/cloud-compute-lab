package cluster

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"battleworld/protocol"
	"battleworld/storage"
	"battleworld/world"
)

type Session struct {
	Username string
	MapID    string
	NodeID   string
	Events   []string
	Version  int64
}

type MapEvents struct {
	MapID  string
	Events []string
}

type NodeService struct {
	mu               sync.RWMutex
	ID               string
	Addr             string
	healthy          bool
	lastHeartbeat    time.Time
	maps             map[string]*world.World
	replicaSnapshots map[string]protocol.MapCheckpoint
	ln               net.Listener
}

type BossState struct {
	Name         string
	HP           int
	MaxHP        int
	Alive        bool
	LastHit      string
	RespawnAt    time.Time
	Sites        []protocol.BossSite
	Version      int64
	Contributors map[string]int
}

type Cluster struct {
	mu       sync.RWMutex
	store    *storage.Store
	nodes    map[string]*NodeService
	sessions map[string]*Session
	owners   map[string]string
	replicas map[string]string
	configs  map[string]world.MapConfig
	boss     *BossState
	stopCh   chan struct{}
}

var studentTodoNotice sync.Map

func NewCluster(store *storage.Store) (*Cluster, error) {
	c := &Cluster{
		store:    store,
		nodes:    make(map[string]*NodeService),
		sessions: make(map[string]*Session),
		owners:   make(map[string]string),
		replicas: make(map[string]string),
		configs:  make(map[string]world.MapConfig),
		boss:     newBossState(),
		stopCh:   make(chan struct{}),
	}

	for _, cfg := range world.AvailableMaps() {
		c.configs[cfg.ID] = cfg
	}
	c.boss.Sites = c.buildBossSites()

	c.nodes["node-a"] = newNodeService("node-a", "127.0.0.1:9311")
	c.nodes["node-b"] = newNodeService("node-b", "127.0.0.1:9312")
	c.nodes["node-c"] = newNodeService("node-c", "127.0.0.1:9313")

	assignments := map[string]struct {
		owner   string
		replica string
	}{
		"green": {owner: "node-a", replica: "node-c"},
		"cave":  {owner: "node-b", replica: "node-c"},
		"ruins": {owner: "node-a", replica: "node-b"},
	}

	for mapID, placement := range assignments {
		cfg := c.configs[mapID]
		c.owners[mapID] = placement.owner
		c.replicas[mapID] = placement.replica
		c.nodes[placement.owner].InstallPrimaryMap(cfg)
		if cp, ok := store.LoadCheckpoint(mapID); ok {
			c.nodes[placement.owner].RestorePrimaryMap(cfg, *cp)
			c.nodes[placement.replica].StoreReplica(*cp)
		}
	}

	return c, nil
}

func (c *Cluster) Start() error {
	for _, node := range c.nodes {
		if err := node.Start(); err != nil {
			return err
		}
	}
	go c.backgroundLoop()
	go c.heartbeatLoop()
	go c.checkpointLoop()
	go c.flushLoop()
	return nil
}

func (c *Cluster) Register(username, password, confirm string) error {
	if confirm == "" {
		return errors.New("注册时必须再次确认密码")
	}
	if password != confirm {
		return errors.New("两次输入的密码不一致")
	}
	return c.store.Register(username, password)
}

func (c *Cluster) ExecuteAdmin(action, nodeID string) (string, error) {
	switch action {
	case "status", "状态":
		return c.adminStatus(), nil
	case "fail", "down", "故障":
		if nodeID == "" {
			return "", errors.New("请指定需要模拟故障的节点")
		}
		return c.failNode(nodeID)
	case "recover", "up", "恢复":
		if nodeID == "" {
			return "", errors.New("请指定需要恢复的节点")
		}
		return c.recoverNode(nodeID)
	default:
		return "", fmt.Errorf("未知管理动作：%s", action)
	}
}

func (c *Cluster) QuickEnter(username, password string) (*protocol.WorldState, error) {
	if err := c.Register(username, password, password); err != nil {
		if !strings.Contains(err.Error(), "已存在") {
			return nil, err
		}
	}
	return c.Login(username, password)
}

func (c *Cluster) Login(username, password string) (*protocol.WorldState, error) {
	profile, err := c.store.Authenticate(username, password)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if _, ok := c.sessions[username]; ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("用户 %q 已经在线", username)
	}
	mapID := profile.LastMap
	if _, ok := c.configs[mapID]; !ok {
		mapID = world.DefaultMapID()
	}
	ownerID := c.owners[mapID]
	owner := c.nodes[ownerID]
	c.mu.Unlock()

	if owner == nil {
		return nil, errors.New("目标地图当前没有可用节点")
	}

	owner.AddPlayer(mapID, profile)

	c.mu.Lock()
	session := &Session{
		Username: username,
		MapID:    mapID,
		NodeID:   ownerID,
		Version:  1,
		Events: []string{
			fmt.Sprintf("欢迎回来，%s", username),
			fmt.Sprintf("当前地图 %s 由 %s 承载", mapID, ownerID),
		},
	}
	c.sessions[username] = session
	c.mu.Unlock()

	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) Logout(username string) error {
	c.mu.Lock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	delete(c.sessions, username)
	c.mu.Unlock()

	node := c.nodes[session.NodeID]
	if node == nil {
		return c.store.DeleteHotSession(username)
	}
	profile, removed := node.RemovePlayer(session.MapID, username)
	if removed {
		profile.LastNode = session.NodeID
		profile.LastMap = session.MapID
		_ = c.store.SaveProfile(profile)
	}
	return c.store.DeleteHotSession(username)
}

func (c *Cluster) Move(username, dir string) (*protocol.WorldState, error) {
	session, node, err := c.sessionNode(username)
	if err != nil {
		return nil, err
	}
	event, profile, ok := node.MovePlayer(session.MapID, username, dir)
	if !ok {
		return nil, errors.New("移动请求被拒绝")
	}
	c.pushEvent(username, event)
	profile.LastNode = session.NodeID
	profile.LastMap = session.MapID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) Attack(username string) (*protocol.WorldState, error) {
	session, node, err := c.sessionNode(username)
	if err != nil {
		return nil, err
	}
	event, targetUsername, targetEvent, profile, ok := node.Attack(session.MapID, username)
	if !ok {
		return nil, errors.New("攻击请求被拒绝")
	}
	c.pushEvent(username, event)
	if targetUsername != "" && targetEvent != "" {
		c.pushEvent(targetUsername, targetEvent)
		if targetProfile, ok := node.Profile(session.MapID, targetUsername); ok {
			targetProfile.LastNode = session.NodeID
			targetProfile.LastMap = session.MapID
			_ = c.store.SaveProfile(targetProfile)
			_ = c.persistSessionState(targetUsername)
		}
	}
	profile.LastNode = session.NodeID
	profile.LastMap = session.MapID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) Heal(username string) (*protocol.WorldState, error) {
	session, node, err := c.sessionNode(username)
	if err != nil {
		return nil, err
	}
	event, profile, ok := node.Heal(session.MapID, username)
	if !ok {
		return nil, errors.New("治疗请求被拒绝")
	}
	c.pushEvent(username, event)
	profile.LastNode = session.NodeID
	profile.LastMap = session.MapID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) BuyItem(username, item string) (*protocol.WorldState, error) {
	session, node, err := c.sessionNode(username)
	if err != nil {
		return nil, err
	}
	event, profile, ok := node.BuyItem(session.MapID, username, item)
	if !ok {
		return nil, errors.New("商店请求被拒绝")
	}
	c.pushEvent(username, event)
	profile.LastNode = session.NodeID
	profile.LastMap = session.MapID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) AttackBoss(username string) (*protocol.WorldState, error) {
	session, node, err := c.sessionNode(username)
	if err != nil {
		return nil, err
	}

	profile, ok := node.Profile(session.MapID, username)
	if !ok {
		return nil, errors.New("当前角色不在地图中")
	}
	if !profile.Alive {
		c.pushEvent(username, "倒地状态下无法攻击世界首领")
		return c.SnapshotFor(username)
	}

	site, ok := c.bossSite(session.MapID)
	if !ok {
		return nil, fmt.Errorf("地图 %q 当前没有首领投影", session.MapID)
	}
	if manhattan(profile.X, profile.Y, site.X, site.Y) > protocol.BossAtkRange {
		c.pushEvent(username, "距离世界首领投影过远，无法造成伤害")
		return c.SnapshotFor(username)
	}

	damage := profile.Attack + profile.Treasures/2
	if damage < 20 {
		damage = 20
	}

	type rewardItem struct {
		username string
		profile  protocol.UserProfile
	}
	rewards := make([]rewardItem, 0, 8)
	needRespawn := false

	c.mu.Lock()
	if !c.boss.Alive {
		c.mu.Unlock()
		c.pushEvent(username, fmt.Sprintf("世界首领【%s】尚未重生，请稍后再战", c.boss.Name))
		return c.SnapshotFor(username)
	}

	c.boss.HP = max(0, c.boss.HP-damage)
	c.boss.LastHit = username
	c.boss.Version++
	c.boss.Contributors[username] += damage

	c.broadcastGlobalEventLocked(fmt.Sprintf("%s 对世界首领【%s】造成 %d 点伤害（剩余 %d/%d）", username, c.boss.Name, damage, c.boss.HP, c.boss.MaxHP))

	if c.boss.HP == 0 {
		c.boss.Alive = false
		c.boss.RespawnAt = time.Now().Add(15 * time.Second)
		needRespawn = true

		for contributor := range c.boss.Contributors {
			s, ok := c.sessions[contributor]
			if !ok {
				continue
			}
			n := c.nodes[s.NodeID]
			if n == nil {
				continue
			}
			updated, ok := n.RewardPlayer(s.MapID, contributor, 6, 1)
			if !ok {
				continue
			}
			updated.LastMap = s.MapID
			updated.LastNode = s.NodeID
			rewards = append(rewards, rewardItem{username: contributor, profile: updated})
			c.pushEventLocked(s, fmt.Sprintf("你参与讨伐并获得奖励：战利品 +%d，胜场 +%d", 6, 1))
		}

		c.broadcastGlobalEventLocked(fmt.Sprintf("%s 终结了世界首领【%s】，全服参战者获得奖励", username, c.boss.Name))
	}
	c.mu.Unlock()

	for _, item := range rewards {
		_ = c.store.SaveProfile(item.profile)
		_ = c.persistSessionState(item.username)
	}

	if needRespawn {
		go c.respawnBossAfterCooldown()
	}

	return c.SnapshotFor(username)
}

func (c *Cluster) SwitchMap(username, targetMap string) (*protocol.WorldState, error) {
	c.mu.RLock()
	if _, ok := c.configs[targetMap]; !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("未知地图：%s", targetMap)
	}
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	fromMap := session.MapID
	fromNodeID := session.NodeID
	toNodeID := c.owners[targetMap]
	fromNode := c.nodes[fromNodeID]
	toNode := c.nodes[toNodeID]
	c.mu.RUnlock()

	if fromNode == nil || toNode == nil || !toNode.IsHealthy() {
		return nil, errors.New("目标地图当前没有可用节点")
	}

	if fromMap == targetMap {
		c.pushEvent(username, fmt.Sprintf("你已在地图 %s", targetMap))
		return c.SnapshotFor(username)
	}

	currentProfile, ok := fromNode.Profile(fromMap, username)
	if !ok {
		return nil, errors.New("当前角色不在源地图中")
	}
	if !currentProfile.Alive {
		c.pushEvent(username, "复活前不能切换地图")
		return c.SnapshotFor(username)
	}

	profile, removed := fromNode.RemovePlayer(fromMap, username)
	if !removed {
		profile = currentProfile
	}
	// 保留来源地图，便于目标地图按照出生点重定位。
	profile.LastMap = fromMap
	profile.LastNode = fromNodeID
	toNode.AddPlayer(targetMap, &profile)

	c.mu.Lock()
	if live, ok := c.sessions[username]; ok {
		live.MapID = targetMap
		live.NodeID = toNodeID
		c.pushEventLocked(live, fmt.Sprintf("你已切换到地图 %s（承载节点 %s）", targetMap, toNodeID))
	}
	c.mu.Unlock()

	profile.LastMap = targetMap
	profile.LastNode = toNodeID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Cluster) SnapshotFor(username string) (*protocol.WorldState, error) {
	c.mu.RLock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	node := c.nodes[session.NodeID]
	sessionVersion := session.Version
	events := append([]string(nil), session.Events...)
	boss := c.boss.viewLocked()
	owners := make(map[string]string, len(c.owners))
	for k, v := range c.owners {
		owners[k] = v
	}
	nodes := make(map[string]*NodeService, len(c.nodes))
	for k, v := range c.nodes {
		nodes[k] = v
	}
	c.mu.RUnlock()

	if node == nil {
		return nil, errors.New("当前承载节点已不可用")
	}

	mapView, err := node.Snapshot(session.MapID)
	if err != nil {
		return nil, err
	}

	self := protocol.PlayerView{}
	for _, player := range mapView.Players {
		if player.Username == username {
			self = player
			break
		}
	}

	mapIDs := make([]string, 0, len(c.configs))
	for mapID := range c.configs {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)

	mapBriefs := make([]protocol.MapBrief, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		ownerID := owners[mapID]
		host := nodes[ownerID]
		if host == nil {
			continue
		}
		players, npcs, treasures, version, err := host.Counts(mapID)
		if err != nil {
			continue
		}
		cfg := c.configs[mapID]
		mapBriefs = append(mapBriefs, protocol.MapBrief{
			ID:        mapID,
			Name:      cfg.Name,
			NodeID:    ownerID,
			Players:   players,
			NPCs:      npcs,
			Treasures: treasures,
			Version:   version,
			Primary:   true,
			IsCurrent: mapID == session.MapID,
		})
	}

	nodeIDs := make([]string, 0, len(nodes))
	for nodeID := range nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	nodeViews := make([]protocol.NodeView, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		nodeViews = append(nodeViews, nodes[nodeID].View())
	}

	return &protocol.WorldState{
		Self:           self,
		Map:            mapView,
		Maps:           mapBriefs,
		Nodes:          nodeViews,
		Boss:           boss,
		Events:         events,
		SessionVersion: sessionVersion,
	}, nil
}

func (c *Cluster) sessionNode(username string) (*Session, *NodeService, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	session, ok := c.sessions[username]
	if !ok {
		return nil, nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	node := c.nodes[session.NodeID]
	if node == nil {
		return nil, nil, fmt.Errorf("节点 %q 当前不可用", session.NodeID)
	}
	copySession := *session
	return &copySession, node, nil
}

func (c *Cluster) pushEvent(username, event string) {
	if event == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	session, ok := c.sessions[username]
	if !ok {
		return
	}
	c.pushEventLocked(session, event)
}

func (c *Cluster) pushEventLocked(session *Session, event string) {
	session.Events = append(session.Events, event)
	if len(session.Events) > 8 {
		session.Events = session.Events[len(session.Events)-8:]
	}
	session.Version++
}

func (c *Cluster) broadcastGlobalEvent(event string) {
	if event == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastGlobalEventLocked(event)
}

func (c *Cluster) broadcastGlobalEventLocked(event string) {
	for _, session := range c.sessions {
		c.pushEventLocked(session, event)
	}
}

func (c *Cluster) broadcastMapEvent(mapID, event string) {
	if event == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, session := range c.sessions {
		if session.MapID == mapID {
			c.pushEventLocked(session, event)
		}
	}
}

func (c *Cluster) persistSessionState(username string) error {
	c.mu.RLock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return c.store.DeleteHotSession(username)
	}
	mapID := session.MapID
	nodeID := session.NodeID
	sessionVersion := session.Version
	node := c.nodes[nodeID]
	c.mu.RUnlock()

	if node == nil {
		return fmt.Errorf("节点 %q 当前不可用", nodeID)
	}

	profile, ok := node.Profile(mapID, username)
	if !ok {
		return fmt.Errorf("玩家 %q 当前不在地图 %q", username, mapID)
	}
	profile.LastMap = mapID
	profile.LastNode = nodeID

	hot := protocol.HotSession{
		Username:       username,
		MapID:          mapID,
		NodeID:         nodeID,
		X:              profile.X,
		Y:              profile.Y,
		HP:             profile.HP,
		Treasures:      profile.Treasures,
		SessionVersion: sessionVersion,
		UpdatedAt:      time.Now(),
	}
	if err := c.store.SaveHotSession(hot); err != nil {
		return err
	}
	return c.store.SaveProfile(profile)
}

func (c *Cluster) respawnBossAfterCooldown() {
	time.Sleep(15 * time.Second)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.boss = newBossState()
	c.broadcastGlobalEventLocked(fmt.Sprintf("世界首领【%s】重新降临，所有服务器均可参与讨伐", c.boss.Name))
}

func (c *Cluster) backgroundLoop() {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nodeIDs := make([]string, 0, len(c.nodes))
			c.mu.RLock()
			for nodeID, node := range c.nodes {
				if node.IsHealthy() {
					nodeIDs = append(nodeIDs, nodeID)
				}
			}
			sort.Strings(nodeIDs)
			nodes := make([]*NodeService, 0, len(nodeIDs))
			for _, nodeID := range nodeIDs {
				nodes = append(nodes, c.nodes[nodeID])
			}
			c.mu.RUnlock()

			for _, node := range nodes {
				for _, bundle := range node.BackgroundStep() {
					for _, event := range bundle.Events {
						c.broadcastMapEvent(bundle.MapID, event)
					}
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cluster) heartbeatLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nodeIDs := make([]string, 0, len(c.nodes))
			c.mu.RLock()
			for nodeID := range c.nodes {
				nodeIDs = append(nodeIDs, nodeID)
			}
			sort.Strings(nodeIDs)
			nodes := make([]*NodeService, 0, len(nodeIDs))
			for _, nodeID := range nodeIDs {
				nodes = append(nodes, c.nodes[nodeID])
			}
			c.mu.RUnlock()

			for _, node := range nodes {
				healthy := ping(node.Addr)
				wasHealthy := node.SetHealthy(healthy)
				if wasHealthy && !healthy {
					c.handleNodeFailure(node.ID)
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cluster) checkpointLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			mapIDs := make([]string, 0, len(c.owners))
			for mapID := range c.owners {
				mapIDs = append(mapIDs, mapID)
			}
			sort.Strings(mapIDs)

			type plan struct {
				mapID     string
				owner     *NodeService
				replica   *NodeService
				replicaID string
			}
			plans := make([]plan, 0, len(mapIDs))
			for _, mapID := range mapIDs {
				ownerID := c.owners[mapID]
				owner := c.nodes[ownerID]
				if owner == nil || !owner.IsHealthy() {
					continue
				}
				replicaID := c.replicas[mapID]
				replica := c.nodes[replicaID]
				if replica != nil && !replica.IsHealthy() {
					replica = nil
				}
				plans = append(plans, plan{mapID: mapID, owner: owner, replica: replica, replicaID: replicaID})
			}
			c.mu.RUnlock()

			for _, p := range plans {
				cp, err := p.owner.Checkpoint(p.mapID)
				if err != nil {
					continue
				}
				_ = c.store.SaveCheckpoint(cp)
				if p.replica != nil {
					p.replica.StoreReplica(cp)
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cluster) flushLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			usernames := make([]string, 0, len(c.sessions))
			for username := range c.sessions {
				usernames = append(usernames, username)
			}
			c.mu.RUnlock()

			for _, username := range usernames {
				_ = c.persistSessionState(username)
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cluster) handleNodeFailure(nodeID string) {
	type persistedProfile struct {
		username string
		profile  protocol.UserProfile
	}

	persistList := make([]persistedProfile, 0, 16)

	c.mu.Lock()
	failedNode := c.nodes[nodeID]
	if failedNode == nil {
		c.mu.Unlock()
		return
	}

	mapIDs := make([]string, 0, len(c.owners))
	for mapID, ownerID := range c.owners {
		if ownerID == nodeID {
			mapIDs = append(mapIDs, mapID)
		}
	}
	sort.Strings(mapIDs)

	for _, mapID := range mapIDs {
		cfg := c.configs[mapID]
		nextOwnerID := ""
		replicaID := c.replicas[mapID]

		if replicaID != "" {
			if replica := c.nodes[replicaID]; replica != nil && replica.IsHealthy() {
				if err := replica.Promote(mapID, cfg); err == nil {
					nextOwnerID = replicaID
				}
			}
		}

		if nextOwnerID == "" {
			candidateID := c.pickReplicaLocked(nodeID)
			if candidateID != "" {
				candidate := c.nodes[candidateID]
				if err := candidate.Promote(mapID, cfg); err == nil {
					nextOwnerID = candidateID
				} else if cp, ok := c.store.LoadCheckpoint(mapID); ok {
					candidate.RestorePrimaryMap(cfg, *cp)
					nextOwnerID = candidateID
				}
			}
		}

		if nextOwnerID == "" {
			continue
		}

		failedNode.RemoveHostedMap(mapID)
		c.owners[mapID] = nextOwnerID

		nextReplicaID := c.pickReplicaLocked(nextOwnerID)
		c.replicas[mapID] = nextReplicaID

		if promoted := c.nodes[nextOwnerID]; promoted != nil {
			if cp, err := promoted.Checkpoint(mapID); err == nil {
				_ = c.store.SaveCheckpoint(cp)
				if replica := c.nodes[nextReplicaID]; replica != nil && replica.IsHealthy() {
					replica.StoreReplica(cp)
				}
			}
		}

		for _, session := range c.sessions {
			if session.MapID != mapID {
				continue
			}
			session.NodeID = nextOwnerID
			c.pushEventLocked(session, fmt.Sprintf("检测到节点 %s 故障，地图 %s 已切换到 %s", nodeID, mapID, nextOwnerID))
			if newNode := c.nodes[nextOwnerID]; newNode != nil {
				if profile, ok := newNode.Profile(mapID, session.Username); ok {
					profile.LastMap = mapID
					profile.LastNode = nextOwnerID
					persistList = append(persistList, persistedProfile{username: session.Username, profile: profile})
				}
			}
		}

		c.broadcastGlobalEventLocked(fmt.Sprintf("故障切换：地图 %s 已从 %s 迁移到 %s", mapID, nodeID, nextOwnerID))
	}
	c.mu.Unlock()

	for _, item := range persistList {
		_ = c.store.SaveProfile(item.profile)
		_ = c.persistSessionState(item.username)
	}
}

func (c *Cluster) pickReplicaLocked(ownerID string) string {
	nodeIDs := make([]string, 0, len(c.nodes))
	for nodeID := range c.nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		if nodeID == ownerID {
			continue
		}
		if c.nodes[nodeID].IsHealthy() {
			return nodeID
		}
	}
	return ""
}

func (c *Cluster) adminStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodeIDs := make([]string, 0, len(c.nodes))
	for nodeID := range c.nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)

	lines := []string{"集群状态总览："}
	for _, nodeID := range nodeIDs {
		view := c.nodes[nodeID].View()
		status := "离线"
		if view.Healthy {
			status = "在线"
		}
		lines = append(lines, fmt.Sprintf("- %s %s 主分片=%v 副本=%v", view.ID, status, view.PrimaryMaps, view.ReplicaMaps))
	}
	return strings.Join(lines, "\n")
}

func (c *Cluster) failNode(nodeID string) (string, error) {
	c.mu.RLock()
	node, ok := c.nodes[nodeID]
	replicas := make(map[string]string, len(c.replicas))
	for mapID, replicaID := range c.replicas {
		replicas[mapID] = replicaID
	}
	c.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("节点 %s 不存在", nodeID)
	}
	view := node.View()
	for _, mapID := range view.PrimaryMaps {
		cp, err := node.Checkpoint(mapID)
		if err != nil {
			continue
		}
		_ = c.store.SaveCheckpoint(cp)
		if replica, ok := c.nodes[replicas[mapID]]; ok {
			replica.StoreReplica(cp)
		}
	}
	if err := node.Stop(); err != nil {
		return "", err
	}
	node.SetHealthy(false)
	c.handleNodeFailure(nodeID)
	c.broadcastGlobalEvent(fmt.Sprintf("管理命令：已模拟 %s 故障，集群开始故障转移", nodeID))
	return fmt.Sprintf("节点 %s 已被标记为故障，并触发主从切换", nodeID), nil
}

func (c *Cluster) recoverNode(nodeID string) (string, error) {
	c.mu.RLock()
	node, ok := c.nodes[nodeID]
	c.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("节点 %s 不存在", nodeID)
	}
	if err := node.Start(); err != nil {
		return "", err
	}
	node.SetHealthy(true)

	c.mu.Lock()
	for mapID, ownerID := range c.owners {
		if ownerID == nodeID {
			continue
		}
		if cp, ok := c.store.LoadCheckpoint(mapID); ok {
			node.StoreReplica(*cp)
			c.replicas[mapID] = nodeID
		}
	}
	c.mu.Unlock()

	c.broadcastGlobalEvent(fmt.Sprintf("管理命令：节点 %s 已恢复在线，并重新接管副本同步", nodeID))
	return fmt.Sprintf("节点 %s 已恢复，最新快照已重新装载", nodeID), nil
}

func (c *Cluster) bossSite(mapID string) (protocol.BossSite, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, site := range c.boss.Sites {
		if site.MapID == mapID {
			return site, true
		}
	}
	return protocol.BossSite{}, false
}

func (c *Cluster) buildBossSites() []protocol.BossSite {
	mapIDs := make([]string, 0, len(c.configs))
	for mapID := range c.configs {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)

	sites := make([]protocol.BossSite, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		cfg := c.configs[mapID]
		sites = append(sites, protocol.BossSite{
			MapID: mapID,
			X:     cfg.BossX,
			Y:     cfg.BossY,
		})
	}
	return sites
}

func newNodeService(id, addr string) *NodeService {
	return &NodeService{
		ID:               id,
		Addr:             addr,
		healthy:          true,
		lastHeartbeat:    time.Now(),
		maps:             make(map[string]*world.World),
		replicaSnapshots: make(map[string]protocol.MapCheckpoint),
	}
}

func (n *NodeService) Start() error {
	n.mu.RLock()
	if n.ln != nil {
		n.mu.RUnlock()
		return nil
	}
	n.mu.RUnlock()

	ln, err := net.Listen("tcp", n.Addr)
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
				_, _ = fmt.Fprintf(c, "node=%s ts=%d\n", n.ID, time.Now().UnixNano())
			}(raw)
		}
	}()
	return nil
}

func (n *NodeService) Stop() error {
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

func (n *NodeService) RemoveHostedMap(mapID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.maps, mapID)
}

func (n *NodeService) InstallPrimaryMap(cfg world.MapConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.maps[cfg.ID]; ok {
		return
	}
	n.maps[cfg.ID] = world.NewWorld(cfg)
}

func (n *NodeService) RestorePrimaryMap(cfg world.MapConfig, cp protocol.MapCheckpoint) {
	n.mu.Lock()
	defer n.mu.Unlock()

	instance, ok := n.maps[cfg.ID]
	if !ok {
		instance = world.NewWorld(cfg)
		n.maps[cfg.ID] = instance
	}
	instance.RestoreCheckpoint(cp)
}

func (n *NodeService) AddPlayer(mapID string, profile *protocol.UserProfile) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return
	}
	instance.AddOrRestorePlayer(profile)
}

func (n *NodeService) RemovePlayer(mapID, username string) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.RemovePlayer(username)
}

func (n *NodeService) MovePlayer(mapID, username, dir string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.MovePlayer(username, dir)
}

func (n *NodeService) Attack(mapID, username string) (string, string, string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", "", "", protocol.UserProfile{}, false
	}
	return instance.Attack(username)
}

func (n *NodeService) Heal(mapID, username string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.HealPlayer(username)
}

func (n *NodeService) BuyItem(mapID, username, item string) (string, protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return "", protocol.UserProfile{}, false
	}
	return instance.BuyItem(username, item)
}

func (n *NodeService) Profile(mapID, username string) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.ProfileOf(username)
}

func (n *NodeService) RewardPlayer(mapID, username string, treasureDelta, victoryDelta int) (protocol.UserProfile, bool) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.UserProfile{}, false
	}
	return instance.RewardPlayer(username, treasureDelta, victoryDelta)
}

func (n *NodeService) Snapshot(mapID string) (protocol.MapView, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.MapView{}, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.ID)
	}
	return instance.Snapshot(n.ID), nil
}

func (n *NodeService) Counts(mapID string) (int, int, int, int64, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return 0, 0, 0, 0, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.ID)
	}
	players, npcs, treasures, version := instance.Counts()
	return players, npcs, treasures, version, nil
}

func (n *NodeService) Checkpoint(mapID string) (protocol.MapCheckpoint, error) {
	n.mu.RLock()
	instance := n.maps[mapID]
	n.mu.RUnlock()
	if instance == nil {
		return protocol.MapCheckpoint{}, fmt.Errorf("地图 %q 当前不在节点 %s 上", mapID, n.ID)
	}
	return instance.CaptureCheckpoint(n.ID), nil
}

func (n *NodeService) BackgroundStep() []MapEvents {
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

func (n *NodeService) StoreReplica(cp protocol.MapCheckpoint) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.replicaSnapshots[cp.MapID] = cp
}

func (n *NodeService) Promote(mapID string, cfg world.MapConfig) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cp, ok := n.replicaSnapshots[mapID]
	if !ok {
		return fmt.Errorf("节点 %s 上没有地图 %q 的副本快照", n.ID, mapID)
	}
	instance := world.NewWorld(cfg)
	instance.RestoreCheckpoint(cp)
	n.maps[mapID] = instance
	delete(n.replicaSnapshots, mapID)
	return nil
}

func (n *NodeService) View() protocol.NodeView {
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
		ID:            n.ID,
		Addr:          n.Addr,
		Healthy:       n.healthy,
		PrimaryMaps:   primaryMaps,
		ReplicaMaps:   replicaMaps,
		LastHeartbeat: n.lastHeartbeat.Format(time.RFC3339),
	}
}

func (n *NodeService) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}

func (n *NodeService) SetHealthy(healthy bool) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	wasHealthy := n.healthy
	n.healthy = healthy
	if healthy {
		n.lastHeartbeat = time.Now()
	}
	return wasHealthy
}

func newBossState() *BossState {
	return &BossState{
		Name:  "烬灭魔龙",
		HP:    1600,
		MaxHP: 1600,
		Alive: true,
		Sites: []protocol.BossSite{
			{MapID: "green", X: 50, Y: 20},
			{MapID: "cave", X: 49, Y: 20},
			{MapID: "ruins", X: 50, Y: 20},
		},
		Version:      1,
		Contributors: make(map[string]int),
	}
}

func (b *BossState) viewLocked() protocol.BossView {
	respawnIn := 0
	if !b.Alive {
		respawnIn = int(time.Until(b.RespawnAt).Seconds())
		if respawnIn < 1 {
			respawnIn = 1
		}
	}
	return protocol.BossView{
		Name:      b.Name,
		HP:        b.HP,
		MaxHP:     b.MaxHP,
		Alive:     b.Alive,
		LastHit:   b.LastHit,
		RespawnIn: respawnIn,
		AttackGap: protocol.BossAtkRange,
		Sites:     append([]protocol.BossSite(nil), b.Sites...),
		Version:   b.Version,
	}
}

func ping(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func manhattan(ax, ay, bx, by int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dy := ay - by
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func studentTODOError(label, funcName, detail string) error {
	logStudentTODO(label, funcName, detail)
	return fmt.Errorf("[%s] TODO 未实现：%s，需要%s", label, funcName, detail)
}

func logStudentTODO(label, funcName, detail string) {
	if _, loaded := studentTodoNotice.LoadOrStore(label, struct{}{}); loaded {
		return
	}
	fmt.Fprintf(os.Stderr, "[%s] student 待实现函数被触发：%s，需要%s\n", label, funcName, detail)
}

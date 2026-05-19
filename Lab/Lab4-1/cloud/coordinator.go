package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"battleworld/cloudapi"
	"battleworld/protocol"
	"battleworld/storage"
	"battleworld/twopc"
	"battleworld/world"
)

type Session struct {
	Username string
	MapID    string
	NodeID   string
	Events   []string
	Version  int64
}

type Store interface {
	Register(username, password string) error
	Authenticate(username, password string) (*protocol.UserProfile, error)
	SaveProfile(profile protocol.UserProfile) error
	SaveHotSession(session protocol.HotSession) error
	DeleteHotSession(username string) error
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

type Coordinator struct {
	mu       sync.RWMutex
	store    Store
	maps     map[string]*MapClient
	configs  map[string]world.MapConfig
	sessions map[string]*Session
	boss     *BossState
	tx       *twopc.Coordinator
	stopCh   chan struct{}
}

func NewCoordinator(store Store, maps map[string]*MapClient) *Coordinator {
	configs := make(map[string]world.MapConfig)
	for _, cfg := range world.AvailableMaps() {
		configs[cfg.ID] = cfg
	}
	return &Coordinator{
		store:    store,
		maps:     maps,
		configs:  configs,
		sessions: make(map[string]*Session),
		boss:     newBossState(configs),
		tx:       &twopc.Coordinator{},
		stopCh:   make(chan struct{}),
	}
}

func NewDefaultStore(root string) (*storage.Store, error) { return storage.NewStore(root) }

func (c *Coordinator) Start() {
	go c.backgroundLoop()
	go c.flushLoop()
}

func (c *Coordinator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, cloudapi.CoordinatorResponse{OK: true})
	})
	mux.HandleFunc("/v1/coordinator", c.handleCoordinator)
	return mux
}

func (c *Coordinator) handleCoordinator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req cloudapi.CoordinatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, cloudapi.CoordinatorResponse{OK: false, Error: err.Error()})
		return
	}

	var (
		state *protocol.WorldState
		text  string
		err   error
	)
	switch req.Action {
	case cloudapi.CoordinatorActionRegister:
		err = c.Register(req.Username, req.Password, req.Confirm)
	case cloudapi.CoordinatorActionLogin:
		state, err = c.Login(req.Username, req.Password)
	case cloudapi.CoordinatorActionQuick:
		state, err = c.QuickEnter(req.Username, req.Password)
	case cloudapi.CoordinatorActionMove:
		state, err = c.Move(req.Username, req.Dir)
	case cloudapi.CoordinatorActionAttack:
		state, err = c.Attack(req.Username)
	case cloudapi.CoordinatorActionBoss:
		state, err = c.AttackBoss(req.Username)
	case cloudapi.CoordinatorActionHeal:
		state, err = c.Heal(req.Username)
	case cloudapi.CoordinatorActionShop:
		state, err = c.BuyItem(req.Username, req.Item)
	case cloudapi.CoordinatorActionSwitchMap:
		state, err = c.SwitchMap(req.Username, req.MapID)
	case cloudapi.CoordinatorActionTransfer:
		err = c.TransferTreasures(req.Username, req.Target, req.Amount)
		if err == nil {
			state, err = c.SnapshotFor(req.Username)
		}
	case cloudapi.CoordinatorActionSnapshot:
		state, err = c.SnapshotFor(req.Username)
	case cloudapi.CoordinatorActionLogout:
		err = c.Logout(req.Username)
	case cloudapi.CoordinatorActionAdmin:
		text, err = c.Admin(req.AdminAction, req.NodeID)
	default:
		err = fmt.Errorf("未知协调器动作 %s", req.Action)
	}
	if err != nil {
		respondJSON(w, cloudapi.CoordinatorResponse{OK: false, Error: err.Error()})
		return
	}
	respondJSON(w, cloudapi.CoordinatorResponse{OK: true, Text: text, State: state})
}

func (c *Coordinator) Register(username, password, confirm string) error {
	if confirm == "" {
		return errors.New("注册时必须再次确认密码")
	}
	if password != confirm {
		return errors.New("两次输入的密码不一致")
	}
	return c.store.Register(username, password)
}

func (c *Coordinator) QuickEnter(username, password string) (*protocol.WorldState, error) {
	if err := c.Register(username, password, password); err != nil && !strings.Contains(err.Error(), "已存在") {
		return nil, err
	}
	return c.Login(username, password)
}

func (c *Coordinator) Login(username, password string) (*protocol.WorldState, error) {
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
	client := c.maps[mapID]
	c.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("地图 %s 当前无服务", mapID)
	}

	profile.LastMap = mapID
	profile.LastNode = client.NodeID
	if _, err := client.AddOrRestorePlayer(profile); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.sessions[username] = &Session{
		Username: username,
		MapID:    mapID,
		NodeID:   client.NodeID,
		Version:  1,
		Events: []string{
			fmt.Sprintf("欢迎回来，%s", username),
			fmt.Sprintf("当前地图 %s 由 %s 承载", mapID, client.NodeID),
		},
	}
	c.mu.Unlock()

	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) Logout(username string) error {
	c.mu.Lock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	delete(c.sessions, username)
	c.mu.Unlock()

	client := c.maps[session.MapID]
	if client != nil {
		if profile, err := client.RemovePlayer(username); err == nil {
			profile.LastMap = session.MapID
			profile.LastNode = session.NodeID
			_ = c.store.SaveProfile(profile)
		}
	}
	return c.store.DeleteHotSession(username)
}

func (c *Coordinator) Move(username, dir string) (*protocol.WorldState, error) {
	session, client, err := c.sessionMap(username)
	if err != nil {
		return nil, err
	}
	event, profile, err := client.MovePlayer(username, dir)
	if err != nil {
		return nil, err
	}
	c.pushEvent(username, event)
	profile.LastMap = session.MapID
	profile.LastNode = session.NodeID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) Attack(username string) (*protocol.WorldState, error) {
	session, client, err := c.sessionMap(username)
	if err != nil {
		return nil, err
	}
	event, targetUsername, targetEvent, profile, err := client.Attack(username)
	if err != nil {
		return nil, err
	}
	c.pushEvent(username, event)
	if targetUsername != "" && targetEvent != "" {
		c.pushEvent(targetUsername, targetEvent)
		if targetProfile, err := client.Profile(targetUsername); err == nil {
			targetProfile.LastMap = session.MapID
			targetProfile.LastNode = session.NodeID
			_ = c.store.SaveProfile(targetProfile)
			_ = c.persistSessionState(targetUsername)
		}
	}
	profile.LastMap = session.MapID
	profile.LastNode = session.NodeID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) Heal(username string) (*protocol.WorldState, error) {
	session, client, err := c.sessionMap(username)
	if err != nil {
		return nil, err
	}
	event, profile, err := client.Heal(username)
	if err != nil {
		return nil, err
	}
	c.pushEvent(username, event)
	profile.LastMap = session.MapID
	profile.LastNode = session.NodeID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) BuyItem(username, item string) (*protocol.WorldState, error) {
	session, client, err := c.sessionMap(username)
	if err != nil {
		return nil, err
	}
	event, profile, err := client.BuyItem(username, item)
	if err != nil {
		return nil, err
	}
	c.pushEvent(username, event)
	profile.LastMap = session.MapID
	profile.LastNode = session.NodeID
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) SwitchMap(username, targetMap string) (*protocol.WorldState, error) {
	c.mu.RLock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	if _, ok := c.configs[targetMap]; !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("地图 %q 不存在", targetMap)
	}
	if session.MapID == targetMap {
		c.mu.RUnlock()
		c.pushEvent(username, fmt.Sprintf("你已经在地图 %s 了", targetMap))
		return c.SnapshotFor(username)
	}
	source := c.maps[session.MapID]
	target := c.maps[targetMap]
	c.mu.RUnlock()
	if source == nil || target == nil {
		return nil, errors.New("地图路由不完整")
	}
	profile, err := source.Profile(username)
	if err != nil {
		return nil, err
	}
	if !profile.Alive {
		c.pushEvent(username, fmt.Sprintf("%s 当前倒地，复活前不能切换地图", username))
		return c.SnapshotFor(username)
	}
	profile, err = source.RemovePlayer(username)
	if err != nil {
		return nil, err
	}
	cfg := c.configs[targetMap]
	profile.LastMap = targetMap
	profile.LastNode = target.NodeID
	profile.X = cfg.SpawnX
	profile.Y = cfg.SpawnY
	if _, err := target.AddOrRestorePlayer(&profile); err != nil {
		return nil, err
	}
	c.mu.Lock()
	session.MapID = targetMap
	session.NodeID = target.NodeID
	session.Version++
	c.pushEventLocked(session, fmt.Sprintf("已切换到地图 %s，当前节点 %s", targetMap, target.NodeID))
	c.mu.Unlock()
	_ = c.store.SaveProfile(profile)
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) SnapshotFor(username string) (*protocol.WorldState, error) {
	c.mu.RLock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	client := c.maps[session.MapID]
	events := append([]string(nil), session.Events...)
	sessionVersion := session.Version
	boss := c.boss.viewLocked()
	online := make([]protocol.OnlinePlayerView, 0, len(c.sessions))
	for _, other := range c.sessions {
		online = append(online, protocol.OnlinePlayerView{Username: other.Username, MapID: other.MapID, NodeID: other.NodeID})
	}
	sort.Slice(online, func(i, j int) bool { return online[i].Username < online[j].Username })
	configs := make(map[string]world.MapConfig, len(c.configs))
	for k, v := range c.configs {
		configs[k] = v
	}
	clients := make(map[string]*MapClient, len(c.maps))
	for k, v := range c.maps {
		clients[k] = v
	}
	c.mu.RUnlock()
	if client == nil {
		return nil, errors.New("当前地图服务不可用")
	}
	mapView, err := client.Snapshot()
	if err != nil {
		return nil, err
	}
	self := protocol.PlayerView{}
	for _, p := range mapView.Players {
		if p.Username == username {
			self = p
			break
		}
	}
	mapIDs := make([]string, 0, len(clients))
	for mapID := range clients {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)
	nodeViews := make([]protocol.NodeView, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		nodeViews = append(nodeViews, clients[mapID].View())
	}
	briefs := MapBriefsFromClients(configs, clients, session.MapID)
	return &protocol.WorldState{
		Self:           self,
		Map:            mapView,
		Maps:           briefs,
		Nodes:          nodeViews,
		Boss:           boss,
		OnlinePlayers:  online,
		Meta:           protocol.MetaView{LeaderID: "coordinator", HealthyNodes: len(clients) + 1},
		Events:         events,
		SessionVersion: sessionVersion,
	}, nil
}

func (c *Coordinator) AttackBoss(username string) (*protocol.WorldState, error) {
	session, client, err := c.sessionMap(username)
	if err != nil {
		return nil, err
	}
	profile, err := client.Profile(username)
	if err != nil {
		return nil, err
	}
	site, ok := c.bossSite(session.MapID)
	if !ok {
		return nil, errors.New("当前地图没有世界首领投影")
	}
	if manhattan(profile.X, profile.Y, site.X, site.Y) > protocol.BossAtkRange {
		c.pushEvent(username, fmt.Sprintf("离世界首领太远，需靠近到 %d 格内才能发起进攻", protocol.BossAtkRange))
		return c.SnapshotFor(username)
	}
	damage := max(20, profile.Attack+profile.Treasures/2)
	now := time.Now()
	type rewardPlan struct {
		username  string
		mapID     string
		treasure  int
		victories int
	}
	var rewards []rewardPlan
	var needRespawn bool
	c.mu.Lock()
	if !c.boss.Alive {
		waitSec := int(time.Until(c.boss.RespawnAt).Seconds())
		if waitSec < 1 {
			waitSec = 1
		}
		if current, ok := c.sessions[username]; ok {
			c.pushEventLocked(current, fmt.Sprintf("世界首领仍在重组，还需 %d 秒再战", waitSec))
		}
		c.mu.Unlock()
		return c.SnapshotFor(username)
	}
	c.boss.HP -= damage
	c.boss.LastHit = username
	c.boss.Version++
	if c.boss.Contributors == nil {
		c.boss.Contributors = make(map[string]int)
	}
	c.boss.Contributors[username] += damage
	if c.boss.HP <= 0 {
		c.boss.HP = 0
		c.boss.Alive = false
		c.boss.RespawnAt = now.Add(15 * time.Second)
		c.broadcastGlobalEventLocked(fmt.Sprintf("世界首领【%s】被 %s 终结，全服共享宝物结算中", c.boss.Name, username))
		for contributor := range c.boss.Contributors {
			if s, ok := c.sessions[contributor]; ok {
				reward := 3
				victories := 0
				if contributor == username {
					reward = 6
					victories = 1
				}
				rewards = append(rewards, rewardPlan{username: contributor, mapID: s.MapID, treasure: reward, victories: victories})
				c.pushEventLocked(s, fmt.Sprintf("你参与了世界首领奖励结算，获得战利品 +%d", reward))
			}
		}
		c.boss.Contributors = make(map[string]int)
		needRespawn = true
	} else {
		c.broadcastGlobalEventLocked(fmt.Sprintf("世界首领【%s】遭到 %s 重击 %d 点，剩余 %d/%d", c.boss.Name, username, damage, c.boss.HP, c.boss.MaxHP))
	}
	c.mu.Unlock()
	for _, reward := range rewards {
		host := c.maps[reward.mapID]
		if host == nil {
			continue
		}
		updated, err := host.RewardPlayer(reward.username, reward.treasure, reward.victories)
		if err != nil {
			continue
		}
		updated.LastMap = reward.mapID
		updated.LastNode = host.NodeID
		_ = c.store.SaveProfile(updated)
	}
	if needRespawn {
		go c.respawnBossAfterCooldown()
	}
	_ = c.persistSessionState(username)
	return c.SnapshotFor(username)
}

func (c *Coordinator) Admin(action, nodeID string) (string, error) {
	switch action {
	case "", "status", "状态":
		return c.adminStatus(), nil
	default:
		return "", fmt.Errorf("云上拆分版暂未实现管理动作 %s", action)
	}
}

func (c *Coordinator) TransferTreasures(fromUsername, toUsername string, amount int) error {
	if amount <= 0 {
		return fmt.Errorf("转移战利品数量必须为正数")
	}
	fromSession, fromMap, err := c.sessionMap(fromUsername)
	if err != nil {
		return err
	}
	toSession, toMap, err := c.sessionMap(toUsername)
	if err != nil {
		return err
	}
	txID := fmt.Sprintf("trade:%s:%s:%d:%d", fromUsername, toUsername, amount, time.Now().UnixNano())
	if err := c.tx.TransferWithParticipants(txID, &treasureParticipant{client: fromMap}, fromUsername, &treasureParticipant{client: toMap}, toUsername, amount); err != nil {
		return err
	}
	if profile, err := fromMap.Profile(fromUsername); err == nil {
		profile.LastMap = fromSession.MapID
		profile.LastNode = fromSession.NodeID
		_ = c.store.SaveProfile(profile)
	}
	if profile, err := toMap.Profile(toUsername); err == nil {
		profile.LastMap = toSession.MapID
		profile.LastNode = toSession.NodeID
		_ = c.store.SaveProfile(profile)
	}
	_ = c.persistSessionState(fromUsername)
	_ = c.persistSessionState(toUsername)
	c.pushEvent(fromUsername, fmt.Sprintf("你向 %s 转移了 %d 份战利品（2PC 已提交）", toUsername, amount))
	c.pushEvent(toUsername, fmt.Sprintf("你从 %s 收到了 %d 份战利品（2PC 已提交）", fromUsername, amount))
	return nil
}

type preparedTreasure struct {
	account string
	delta   int
}

type treasureParticipant struct {
	mu       sync.Mutex
	client   *MapClient
	prepared map[string]preparedTreasure
}

func (p *treasureParticipant) Prepare(txID, account string, delta int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prepared == nil {
		p.prepared = make(map[string]preparedTreasure)
	}
	if _, ok := p.prepared[txID]; ok {
		return nil
	}
	profile, err := p.client.Profile(account)
	if err != nil {
		return err
	}
	if profile.Treasures+delta < 0 {
		return fmt.Errorf("用户 %s 战利品不足", account)
	}
	p.prepared[txID] = preparedTreasure{account: account, delta: delta}
	return nil
}

func (p *treasureParticipant) Commit(txID string) error {
	p.mu.Lock()
	pending, ok := p.prepared[txID]
	if ok {
		delete(p.prepared, txID)
	}
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("事务 %s 未 prepare", txID)
	}
	_, err := p.client.AdjustTreasures(pending.account, pending.delta)
	return err
}

func (p *treasureParticipant) Abort(txID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prepared != nil {
		delete(p.prepared, txID)
	}
	return nil
}

func (c *Coordinator) persistSessionState(username string) error {
	c.mu.RLock()
	session, ok := c.sessions[username]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	mapID, nodeID, version := session.MapID, session.NodeID, session.Version
	client := c.maps[mapID]
	c.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("地图 %s 当前不可用", mapID)
	}
	profile, err := client.Profile(username)
	if err != nil {
		return nil
	}
	profile.LastMap = mapID
	profile.LastNode = nodeID
	if err := c.store.SaveProfile(profile); err != nil {
		return err
	}
	return c.store.SaveHotSession(protocol.HotSession{
		Username:       username,
		MapID:          mapID,
		NodeID:         nodeID,
		X:              profile.X,
		Y:              profile.Y,
		HP:             profile.HP,
		Treasures:      profile.Treasures,
		SessionVersion: version,
		UpdatedAt:      time.Now(),
	})
}

func (c *Coordinator) backgroundLoop() {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for mapID, client := range c.maps {
				events, err := client.DrainEvents()
				if err != nil || len(events) == 0 {
					continue
				}
				for _, event := range events {
					c.broadcastMapEvent(mapID, event)
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

func (c *Coordinator) flushLoop() {
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

func (c *Coordinator) sessionMap(username string) (*Session, *MapClient, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, ok := c.sessions[username]
	if !ok {
		return nil, nil, fmt.Errorf("用户 %q 当前不在线", username)
	}
	client := c.maps[session.MapID]
	if client == nil {
		return nil, nil, fmt.Errorf("地图 %s 当前无服务", session.MapID)
	}
	copySession := *session
	return &copySession, client, nil
}

func (c *Coordinator) pushEvent(username, event string) {
	if event == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if session, ok := c.sessions[username]; ok {
		c.pushEventLocked(session, event)
	}
}

func (c *Coordinator) pushEventLocked(session *Session, event string) {
	session.Events = append(session.Events, event)
	if len(session.Events) > 8 {
		session.Events = session.Events[len(session.Events)-8:]
	}
	session.Version++
}

func (c *Coordinator) broadcastGlobalEventLocked(event string) {
	for _, session := range c.sessions {
		c.pushEventLocked(session, event)
	}
}

func (c *Coordinator) broadcastMapEvent(mapID, event string) {
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

func (c *Coordinator) respawnBossAfterCooldown() {
	time.Sleep(15 * time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.boss = newBossState(c.configs)
	c.broadcastGlobalEventLocked(fmt.Sprintf("世界首领【%s】重新降临，所有服务器均可参与讨伐", c.boss.Name))
}

func (c *Coordinator) bossSite(mapID string) (protocol.BossSite, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, site := range c.boss.Sites {
		if site.MapID == mapID {
			return site, true
		}
	}
	return protocol.BossSite{}, false
}

func (c *Coordinator) adminStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mapIDs := make([]string, 0, len(c.maps))
	for mapID := range c.maps {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)
	lines := []string{"云上拆分版协调器状态："}
	for _, mapID := range mapIDs {
		client := c.maps[mapID]
		lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", mapID, client.NodeID, client.BaseURL))
	}
	return strings.Join(lines, "\n")
}

func newBossState(configs map[string]world.MapConfig) *BossState {
	sites := make([]protocol.BossSite, 0, len(configs))
	mapIDs := make([]string, 0, len(configs))
	for mapID := range configs {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)
	for _, mapID := range mapIDs {
		cfg := configs[mapID]
		sites = append(sites, protocol.BossSite{MapID: mapID, X: cfg.BossX, Y: cfg.BossY})
	}
	return &BossState{Name: "烬灭魔龙", HP: 1600, MaxHP: 1600, Alive: true, Sites: sites, Version: 1, Contributors: make(map[string]int)}
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

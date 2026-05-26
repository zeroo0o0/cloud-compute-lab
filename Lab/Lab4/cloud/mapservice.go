package cloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"battleworld/cloudapi"
	"battleworld/protocol"
	"battleworld/world"
)

type MapService struct {
	mu            sync.Mutex
	nodeID        string
	world         *world.World
	events        []string
	afterMutation func()
}

func NewMapService(nodeID, mapID string) (*MapService, error) {
	cfg, ok := world.FindConfig(mapID)
	if !ok {
		return nil, fmt.Errorf("未知地图 %s", mapID)
	}
	return &MapService{nodeID: nodeID, world: world.NewWorld(cfg)}, nil
}

func (m *MapService) StartBackground(stop <-chan struct{}) {
	m.StartBackgroundWhen(stop, func() bool { return true })
}

func (m *MapService) StartBackgroundWhen(stop <-chan struct{}, active func() bool) {
	ticker := time.NewTicker(700 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if active != nil && !active() {
					continue
				}
				events := m.world.BackgroundStep()
				if len(events) == 0 {
					continue
				}
				m.mu.Lock()
				m.events = append(m.events, events...)
				if len(m.events) > 64 {
					m.events = m.events[len(m.events)-64:]
				}
				m.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}

func (m *MapService) Checkpoint() protocol.MapCheckpoint {
	return m.world.CaptureCheckpoint(m.nodeID)
}

func (m *MapService) ActivePlayers() int {
	players, _, _, _ := m.world.Counts()
	return players
}

func (m *MapService) RestoreCheckpoint(cp protocol.MapCheckpoint) {
	if cp.MapID != "" && cp.MapID != m.world.MapID() {
		return
	}
	m.world.RestoreCheckpoint(cp)
}

func (m *MapService) SetAfterMutation(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.afterMutation = fn
}

func (m *MapService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, cloudapi.MapResponse{OK: true})
	})
	mux.HandleFunc("/v1/map", m.handleMap)
	return mux
}

func (m *MapService) handleMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req cloudapi.MapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, cloudapi.MapResponse{OK: false, Error: err.Error()})
		return
	}

	var resp cloudapi.MapResponse
	switch req.Action {
	case cloudapi.MapActionAddOrRestore:
		if req.Profile == nil {
			resp = cloudapi.MapResponse{OK: false, Error: "缺少 profile"}
			break
		}
		profile := *req.Profile
		m.world.AddOrRestorePlayer(&profile)
		updated, ok := m.world.ProfileOf(profile.Username)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "恢复玩家后未找到状态"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Profile: &updated}
		m.afterMutationHook()
	case cloudapi.MapActionRemove:
		profile, ok := m.world.RemovePlayer(req.Username)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "未找到玩家"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionMove:
		event, profile, ok := m.world.MovePlayer(req.Username, req.Dir)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "移动失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Event: event, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionAttack:
		event, targetUsername, targetEvent, profile, ok := m.world.Attack(req.Username)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "攻击失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Event: event, TargetUsername: targetUsername, TargetEvent: targetEvent, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionHeal:
		event, profile, ok := m.world.HealPlayer(req.Username)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "治疗失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Event: event, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionBuy:
		event, profile, ok := m.world.BuyItem(req.Username, req.Item)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "商店操作失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Event: event, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionProfile:
		profile, ok := m.world.ProfileOf(req.Username)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "未找到玩家"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Profile: &profile}
	case cloudapi.MapActionReward:
		profile, ok := m.world.RewardPlayer(req.Username, req.TreasureDelta, req.VictoryDelta)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "奖励发放失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionAdjust:
		profile, ok := m.world.AdjustTreasures(req.Username, req.Delta)
		if !ok {
			resp = cloudapi.MapResponse{OK: false, Error: "战利品调整失败"}
			break
		}
		resp = cloudapi.MapResponse{OK: true, Profile: &profile}
		m.afterMutationHook()
	case cloudapi.MapActionSnapshot:
		snapshot := m.world.Snapshot(m.nodeID)
		resp = cloudapi.MapResponse{OK: true, Map: &snapshot}
	case cloudapi.MapActionCounts:
		players, npcs, treasures, version := m.world.Counts()
		resp = cloudapi.MapResponse{OK: true, Counts: &cloudapi.MapCounts{Players: players, NPCs: npcs, Treasures: treasures, Version: version}}
	case cloudapi.MapActionCheckpoint:
		cp := m.world.CaptureCheckpoint(m.nodeID)
		resp = cloudapi.MapResponse{OK: true, Checkpoint: &cp}
	case cloudapi.MapActionDrainEvents:
		m.mu.Lock()
		events := append([]string(nil), m.events...)
		m.events = nil
		m.mu.Unlock()
		resp = cloudapi.MapResponse{OK: true, Bundle: &cloudapi.MapEventBundle{Events: events}}
	case cloudapi.MapActionHealth:
		view := protocol.NodeView{ID: m.nodeID, Addr: "", Healthy: true, Status: "alive", PrimaryMaps: []string{m.world.MapID()}}
		resp = cloudapi.MapResponse{OK: true, Node: &view}
	default:
		resp = cloudapi.MapResponse{OK: false, Error: fmt.Sprintf("未知地图动作 %s", req.Action)}
	}
	respondJSON(w, resp)
}

func (m *MapService) afterMutationHook() {
	m.mu.Lock()
	fn := m.afterMutation
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

type MapClient struct {
	MapID   string
	NodeID  string
	BaseURL string
	client  *http.Client
}

func NewMapClient(mapID, nodeID, baseURL string) *MapClient {
	return &MapClient{MapID: mapID, NodeID: nodeID, BaseURL: cloudapi.NormalizeBaseURL(baseURL), client: cloudapi.NewHTTPClient()}
}

func (c *MapClient) call(req cloudapi.MapRequest) (cloudapi.MapResponse, error) {
	var resp cloudapi.MapResponse
	if err := cloudapi.PostJSON(c.client, c.BaseURL+"/v1/map", req, &resp); err != nil {
		return resp, err
	}
	if !resp.OK {
		return resp, fmt.Errorf(resp.Error)
	}
	return resp, nil
}

func (c *MapClient) AddOrRestorePlayer(profile *protocol.UserProfile) (protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionAddOrRestore, Profile: profile})
	if err != nil {
		return protocol.UserProfile{}, err
	}
	return *resp.Profile, nil
}

func (c *MapClient) RemovePlayer(username string) (protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionRemove, Username: username})
	if err != nil {
		return protocol.UserProfile{}, err
	}
	return *resp.Profile, nil
}

func (c *MapClient) MovePlayer(username, dir string) (string, protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionMove, Username: username, Dir: dir})
	if err != nil {
		return "", protocol.UserProfile{}, err
	}
	return resp.Event, *resp.Profile, nil
}

func (c *MapClient) Attack(username string) (string, string, string, protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionAttack, Username: username})
	if err != nil {
		return "", "", "", protocol.UserProfile{}, err
	}
	return resp.Event, resp.TargetUsername, resp.TargetEvent, *resp.Profile, nil
}

func (c *MapClient) Heal(username string) (string, protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionHeal, Username: username})
	if err != nil {
		return "", protocol.UserProfile{}, err
	}
	return resp.Event, *resp.Profile, nil
}

func (c *MapClient) BuyItem(username, item string) (string, protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionBuy, Username: username, Item: item})
	if err != nil {
		return "", protocol.UserProfile{}, err
	}
	return resp.Event, *resp.Profile, nil
}

func (c *MapClient) Profile(username string) (protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionProfile, Username: username})
	if err != nil {
		return protocol.UserProfile{}, err
	}
	return *resp.Profile, nil
}

func (c *MapClient) RewardPlayer(username string, treasureDelta, victoryDelta int) (protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionReward, Username: username, TreasureDelta: treasureDelta, VictoryDelta: victoryDelta})
	if err != nil {
		return protocol.UserProfile{}, err
	}
	return *resp.Profile, nil
}

func (c *MapClient) AdjustTreasures(username string, delta int) (protocol.UserProfile, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionAdjust, Username: username, Delta: delta})
	if err != nil {
		return protocol.UserProfile{}, err
	}
	return *resp.Profile, nil
}

func (c *MapClient) Snapshot() (protocol.MapView, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionSnapshot})
	if err != nil {
		return protocol.MapView{}, err
	}
	return *resp.Map, nil
}

func (c *MapClient) Counts() (cloudapi.MapCounts, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionCounts})
	if err != nil {
		return cloudapi.MapCounts{}, err
	}
	return *resp.Counts, nil
}

func (c *MapClient) Checkpoint() (protocol.MapCheckpoint, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionCheckpoint})
	if err != nil {
		return protocol.MapCheckpoint{}, err
	}
	return *resp.Checkpoint, nil
}

func (c *MapClient) DrainEvents() ([]string, error) {
	resp, err := c.call(cloudapi.MapRequest{Action: cloudapi.MapActionDrainEvents})
	if err != nil {
		return nil, err
	}
	if resp.Bundle == nil {
		return nil, nil
	}
	return resp.Bundle.Events, nil
}

func (c *MapClient) View() protocol.NodeView {
	counts, err := c.Counts()
	if err != nil {
		return protocol.NodeView{ID: c.NodeID, Addr: c.BaseURL, Healthy: false, Status: "dead"}
	}
	return protocol.NodeView{ID: c.NodeID, Addr: c.BaseURL, Healthy: true, Status: "alive", PrimaryMaps: []string{c.MapID}, LastHeartbeat: time.Now().Format(time.RFC3339), ReplicaMaps: []string{fmt.Sprintf("players=%d npcs=%d t=%d", counts.Players, counts.NPCs, counts.Treasures)}}
}

func MapBriefsFromClients(configs map[string]world.MapConfig, clients map[string]*MapClient, currentMap string) []protocol.MapBrief {
	mapIDs := make([]string, 0, len(configs))
	for mapID := range configs {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Strings(mapIDs)
	briefs := make([]protocol.MapBrief, 0, len(mapIDs))
	for _, mapID := range mapIDs {
		client := clients[mapID]
		cfg := configs[mapID]
		if client == nil {
			continue
		}
		counts, err := client.Counts()
		if err != nil {
			briefs = append(briefs, protocol.MapBrief{ID: mapID, Name: cfg.Name, NodeID: client.NodeID, IsCurrent: mapID == currentMap})
			continue
		}
		briefs = append(briefs, protocol.MapBrief{ID: mapID, Name: cfg.Name, NodeID: client.NodeID, Players: counts.Players, NPCs: counts.NPCs, Treasures: counts.Treasures, Version: counts.Version, Primary: true, IsCurrent: mapID == currentMap})
	}
	return briefs
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

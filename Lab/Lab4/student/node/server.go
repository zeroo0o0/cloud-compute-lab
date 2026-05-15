package node

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"battleworld/protocol"
	"battleworld/world"
)

// Server 游戏节点 RPC 服务器
type Server struct {
	mu      sync.RWMutex
	id      string
	addr    string
	maps    map[string]*world.World
	ln      net.Listener
	healthy bool
}

// NewServer 创建节点服务器
func NewServer(id, addr string) *Server {
	return &Server{
		id:      id,
		addr:    addr,
		maps:    make(map[string]*world.World),
		healthy: true,
	}
}

// InstallMap 安装地图
func (s *Server) InstallMap(cfg world.MapConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.maps[cfg.ID]; !ok {
		s.maps[cfg.ID] = world.NewWorld(cfg)
	}
}

// RestoreMap 从检查点恢复地图
func (s *Server) RestoreMap(cfg world.MapConfig, cp protocol.MapCheckpoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	instance, ok := s.maps[cfg.ID]
	if !ok {
		instance = world.NewWorld(cfg)
		s.maps[cfg.ID] = instance
	}
	instance.RestoreCheckpoint(cp)
}

// Start 启动节点服务器
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	s.healthy = true
	s.mu.Unlock()

	fmt.Fprintf(os.Stderr, "[node-%s] 节点服务器已启动: %s\n", s.id, s.addr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handleConn(conn)
		}
	}()

	// 启动后台循环
	go s.backgroundLoop()

	return nil
}

// Stop 停止节点服务器
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	s.healthy = false
	return err
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req protocol.NodeRequest
	if err := decoder.Decode(&req); err != nil {
		return
	}

	resp := s.dispatch(req)
	if err := encoder.Encode(resp); err != nil {
		return
	}
}

func (s *Server) dispatch(req protocol.NodeRequest) protocol.NodeResponse {
	switch req.Action {
	case protocol.NodeActionPing:
		return protocol.NodeResponse{OK: true}

	case protocol.NodeActionAddPlayer:
		return s.handleAddPlayer(req)

	case protocol.NodeActionRemovePlayer:
		return s.handleRemovePlayer(req)

	case protocol.NodeActionMovePlayer:
		return s.handleMovePlayer(req)

	case protocol.NodeActionAttack:
		return s.handleAttack(req)

	case protocol.NodeActionHeal:
		return s.handleHeal(req)

	case protocol.NodeActionBuyItem:
		return s.handleBuyItem(req)

	case protocol.NodeActionSnapshot:
		return s.handleSnapshot(req)

	case protocol.NodeActionCounts:
		return s.handleCounts(req)

	case protocol.NodeActionCheckpoint:
		return s.handleCheckpoint(req)

	case protocol.NodeActionProfile:
		return s.handleProfile(req)

	case protocol.NodeActionReward:
		return s.handleReward(req)

	default:
		return protocol.NodeResponse{OK: false, Error: fmt.Sprintf("未知动作: %s", req.Action)}
	}
}

func (s *Server) handleAddPlayer(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	instance.AddOrRestorePlayer(req.Profile)
	return protocol.NodeResponse{OK: true}
}

func (s *Server) handleRemovePlayer(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	profile, ok := instance.RemovePlayer(req.Username)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "玩家不存在"}
	}
	return protocol.NodeResponse{OK: true, Profile: &profile}
}

func (s *Server) handleMovePlayer(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	event, profile, ok := instance.MovePlayer(req.Username, req.Dir)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "移动失败"}
	}
	return protocol.NodeResponse{OK: true, Event: event, Profile: &profile}
}

func (s *Server) handleAttack(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	event, target, event2, profile, ok := instance.Attack(req.Username)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "攻击失败"}
	}
	return protocol.NodeResponse{OK: true, Event: event, Target: target, Event2: event2, Profile: &profile}
}

func (s *Server) handleHeal(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	event, profile, ok := instance.HealPlayer(req.Username)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "治疗失败"}
	}
	return protocol.NodeResponse{OK: true, Event: event, Profile: &profile}
}

func (s *Server) handleBuyItem(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	event, profile, ok := instance.BuyItem(req.Username, req.Item)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "购买失败"}
	}
	return protocol.NodeResponse{OK: true, Event: event, Profile: &profile}
}

func (s *Server) handleSnapshot(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	snapshot := instance.Snapshot(s.id)
	return protocol.NodeResponse{OK: true, State: &snapshot}
}

func (s *Server) handleCounts(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	players, npcs, treasures, version := instance.Counts()
	return protocol.NodeResponse{OK: true, Players: players, NPCs: npcs, Treasures: treasures, Version: version}
}

func (s *Server) handleCheckpoint(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	cp := instance.CaptureCheckpoint(s.id)
	return protocol.NodeResponse{OK: true, Checkpoint: &cp}
}

func (s *Server) handleProfile(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	profile, ok := instance.ProfileOf(req.Username)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "玩家不存在"}
	}
	return protocol.NodeResponse{OK: true, Profile: &profile}
}

func (s *Server) handleReward(req protocol.NodeRequest) protocol.NodeResponse {
	s.mu.RLock()
	instance := s.maps[req.MapID]
	s.mu.RUnlock()
	if instance == nil {
		return protocol.NodeResponse{OK: false, Error: "地图不存在"}
	}
	profile, ok := instance.RewardPlayer(req.Username, req.Treasure, req.Victory)
	if !ok {
		return protocol.NodeResponse{OK: false, Error: "奖励失败"}
	}
	return protocol.NodeResponse{OK: true, Profile: &profile}
}

func (s *Server) backgroundLoop() {
	// 节点内部的后台循环由 cluster 层管理
	// 在微服务模式下，节点自行管理 BackgroundStep
	for {
		time.Sleep(700 * time.Millisecond)
		s.mu.RLock()
		for mapID, instance := range s.maps {
			events := instance.BackgroundStep()
			if len(events) > 0 {
				_ = mapID
				_ = events
			}
		}
		s.mu.RUnlock()
	}
}

// LoadMapAssignments 从环境变量加载地图分配
func (s *Server) LoadMapAssignments() {
	assignments := os.Getenv("MAP_ASSIGNMENTS")
	if assignments == "" {
		assignments = "green=node-a:node-c,cave=node-b:node-c,ruins=node-a:node-b"
	}

	maps := make(map[string]bool)
	for _, pair := range strings.Split(assignments, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			mapID := parts[0]
			nodes := strings.SplitN(parts[1], ":", 2)
			if len(nodes) >= 1 && nodes[0] == s.id {
				maps[mapID] = true
			}
			if len(nodes) >= 2 && nodes[1] == s.id {
				maps[mapID] = true
			}
		}
	}

	available := world.AvailableMaps()
	for _, cfg := range available {
		if maps[cfg.ID] {
			s.InstallMap(cfg)
			fmt.Fprintf(os.Stderr, "[node-%s] 已安装地图: %s (%s)\n", s.id, cfg.ID, cfg.Name)
		}
	}
}

// 辅助函数
func sortStrings(s []string) {
	sort.Strings(s)
}

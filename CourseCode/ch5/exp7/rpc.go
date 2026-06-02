package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ── RPC 请求/响应结构体 ──────────────────────────────────────────────────────

type RequestVoteReq struct {
	Term        int `json:"term"`
	CandidateID int `json:"candidateId"`
}

type RequestVoteResp struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"voteGranted"`
}

type AppendEntriesReq struct {
	Term     int `json:"term"`
	LeaderID int `json:"leaderId"`
}

type AppendEntriesResp struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
}

// ── HTTP Handler：接收 RPC ───────────────────────────────────────────────────

func (n *Node) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	var req RequestVoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	n.mu.Lock()

	resp := RequestVoteResp{Term: n.currentTerm}

	if req.Term < n.currentTerm {
		// 请求的任期过期，拒绝
		resp.VoteGranted = false
		n.mu.Unlock()
	} else {
		// 发现更高任期 → 更新任期，退回 Follower
		if req.Term > n.currentTerm {
			n.currentTerm = req.Term
			n.votedFor = -1
			n.state = Follower
			n.leaderID = -1
		}
		// 本任期未投票，或已投给该 Candidate → 同意
		if n.votedFor == -1 || n.votedFor == req.CandidateID {
			n.votedFor = req.CandidateID
			resp.VoteGranted = true
			resp.Term = n.currentTerm
			n.log("voted for Node %d", req.CandidateID)
			n.mu.Unlock()
			// 投票后重置选举超时
			select {
			case n.resetTimerCh <- struct{}{}:
			default:
			}
		} else {
			resp.VoteGranted = false
			n.log("denied vote for Node %d, already voted for Node %d", req.CandidateID, n.votedFor)
			n.mu.Unlock()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (n *Node) handleAppendEntries(w http.ResponseWriter, r *http.Request) {
	var req AppendEntriesReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	n.mu.Lock()

	resp := AppendEntriesResp{Term: n.currentTerm}

	if req.Term < n.currentTerm {
		// Leader 任期过期，拒绝
		resp.Success = false
		n.mu.Unlock()
	} else {
		// 合法心跳：更新任期，转为 Follower，记录 Leader
		if req.Term > n.currentTerm {
			n.currentTerm = req.Term
			n.votedFor = -1
		}
		n.state = Follower
		n.leaderID = req.LeaderID
		resp.Success = true
		resp.Term = n.currentTerm
		n.mu.Unlock()
		// 收到合法心跳，重置选举超时
		select {
		case n.resetTimerCh <- struct{}{}:
		default:
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── HTTP Client：发送 RPC ───────────────────────────────────────────────────

// httpClient 带超时，避免连接失败时长时间阻塞
var httpClient = &http.Client{Timeout: 100 * time.Millisecond}

func (n *Node) sendRequestVote(peer string, req RequestVoteReq) (RequestVoteResp, error) {
	var resp RequestVoteResp
	body, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	httpResp, err := httpClient.Post("http://"+peer+"/request-vote", "application/json", bytes.NewReader(body))
	if err != nil {
		return resp, fmt.Errorf("rpc to %s failed: %w", peer, err)
	}
	defer httpResp.Body.Close()
	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	return resp, err
}

func (n *Node) sendAppendEntries(peer string, req AppendEntriesReq) (AppendEntriesResp, error) {
	var resp AppendEntriesResp
	body, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	httpResp, err := httpClient.Post("http://"+peer+"/append-entries", "application/json", bytes.NewReader(body))
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()
	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	return resp, err
}

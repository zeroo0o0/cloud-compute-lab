package cluster

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

type Member struct {
	ID     string
	Weight int
}

type Placement struct {
	Primary  string
	Replicas []string
}

type Move struct {
	Key  string
	From string
	To   string
}

type point struct {
	Hash   uint64
	Member string
}

type Ring struct {
	ReplicationFactor int
	VirtualFactor     int
	members           []Member
	points            []point
}

func New(replicationFactor, virtualFactor int) *Ring {
	if replicationFactor < 1 {
		replicationFactor = 1
	}
	if virtualFactor < 1 {
		virtualFactor = 32
	}
	return &Ring{ReplicationFactor: replicationFactor, VirtualFactor: virtualFactor}
}

func (r *Ring) SetMembers(members []Member) error {
	if len(members) == 0 {
		return errors.New("一致性哈希环至少需要一个节点")
	}
	unique := make(map[string]Member, len(members))
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if member.ID == "" {
			return errors.New("节点 ID 不能为空")
		}
		if _, ok := unique[member.ID]; ok {
			return fmt.Errorf("检测到重复节点 %q", member.ID)
		}
		if member.Weight <= 0 {
			member.Weight = 1
		}
		unique[member.ID] = member
		ids = append(ids, member.ID)
	}
	sort.Strings(ids)

	normalized := make([]Member, 0, len(ids))
	points := make([]point, 0)
	for _, id := range ids {
		member := unique[id]
		normalized = append(normalized, member)
		for vnode := 0; vnode < member.Weight*r.VirtualFactor; vnode++ {
			points = append(points, point{Hash: hashRingString(fmt.Sprintf("%s#%d", member.ID, vnode)), Member: member.ID})
		}
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Hash == points[j].Hash {
			return points[i].Member < points[j].Member
		}
		return points[i].Hash < points[j].Hash
	})

	r.members = normalized
	r.points = points
	return nil
}

func (r *Ring) Locate(key string) (Placement, error) {
	if len(r.points) == 0 {
		return Placement{}, errors.New("一致性哈希环尚未初始化成员")
	}
	if key == "" {
		return Placement{}, errors.New("key 不能为空")
	}
	target := hashRingString(key)
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i].Hash >= target })
	if idx == len(r.points) {
		idx = 0
	}
	need := r.ReplicationFactor
	if need > len(r.members) {
		need = len(r.members)
	}
	owners := make([]string, 0, need)
	seen := make(map[string]bool, need)
	for step := 0; step < len(r.points) && len(owners) < need; step++ {
		member := r.points[(idx+step)%len(r.points)].Member
		if seen[member] {
			continue
		}
		seen[member] = true
		owners = append(owners, member)
	}
	if len(owners) == 0 {
		return Placement{}, errors.New("未能从哈希环中找到归属节点")
	}
	placement := Placement{Primary: owners[0]}
	if len(owners) > 1 {
		placement.Replicas = append([]string(nil), owners[1:]...)
	}
	return placement, nil
}

func (r *Ring) RebalancePlan(keys []string, nextMembers []Member) ([]Move, error) {
	if len(r.points) == 0 {
		return nil, errors.New("当前哈希环尚未初始化成员")
	}
	next := New(r.ReplicationFactor, r.VirtualFactor)
	if err := next.SetMembers(nextMembers); err != nil {
		return nil, err
	}
	moves := make([]Move, 0)
	for _, key := range keys {
		before, err := r.Locate(key)
		if err != nil {
			return nil, err
		}
		after, err := next.Locate(key)
		if err != nil {
			return nil, err
		}
		if before.Primary != after.Primary {
			moves = append(moves, Move{Key: key, From: before.Primary, To: after.Primary})
		}
	}
	sort.Slice(moves, func(i, j int) bool {
		if moves[i].Key == moves[j].Key {
			if moves[i].From == moves[j].From {
				return moves[i].To < moves[j].To
			}
			return moves[i].From < moves[j].From
		}
		return moves[i].Key < moves[j].Key
	})
	return moves, nil
}

func hashRingString(text string) uint64 {
	sum := sha1.Sum([]byte(text))
	return binary.BigEndian.Uint64(sum[:8])
}

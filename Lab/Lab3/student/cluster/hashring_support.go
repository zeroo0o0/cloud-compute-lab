package cluster

/*
TODO A-1：一致性哈希主副本定位

这个文件负责“地图应该由哪个节点承载”。

在游戏中的作用：
1. 集群启动时，Cluster 会把 node-a、node-b、node-c 放入哈希环。
2. 每张地图会以 "map:green"、"map:cave"、"map:ruins" 作为 key 查询哈希环。
3. Locate 返回的 Primary 是地图主节点，Replicas 是副本节点。
4. 节点故障或恢复时，RebalancePlan 用于描述哪些地图需要迁移。

你需要完成：
1. Ring.SetMembers：构造虚拟节点哈希环，并按哈希值排序。
2. Ring.Locate：根据 key 找到主节点和不重复副本。
3. Ring.RebalancePlan：比较节点变化前后的主节点，生成最小迁移计划。

关键要求：
1. 同一个真实节点可以有多个虚拟节点。
2. 主节点和副本节点不能重复。
3. 新增或删除节点时，不应该导致所有地图都迁移。
*/

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
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
	return errors.New("[Lab3-A1] TODO 未实现：Ring.SetMembers，需要构建虚拟节点哈希环并排序")
}

func (r *Ring) Locate(key string) (Placement, error) {
	return Placement{}, errors.New("[Lab3-A1] TODO 未实现：Ring.Locate，需要按 key 定位主节点和不重复副本")
}

func (r *Ring) RebalancePlan(keys []string, nextMembers []Member) ([]Move, error) {
	return nil, errors.New("[Lab3-A1] TODO 未实现：Ring.RebalancePlan，需要生成节点变更后的最小迁移计划")
}

func hashRingString(text string) uint64 {
	sum := sha1.Sum([]byte(text))
	return binary.BigEndian.Uint64(sum[:8])
}

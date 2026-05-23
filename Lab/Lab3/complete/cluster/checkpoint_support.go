package cluster

// RunCheckpointOnce 执行一次地图主节点检查点抓取与副本同步。
// 测试会直接调用它，后台 checkpointLoop 也遵循同一套逻辑。
func (c *Cluster) RunCheckpointOnce() error {
	c.mu.RLock()
	owners := make(map[string]string, len(c.owners))
	replicas := make(map[string]string, len(c.replicas))
	nodes := make(map[string]*NodeService, len(c.nodes))
	for k, v := range c.owners {
		owners[k] = v
	}
	for k, v := range c.replicas {
		replicas[k] = v
	}
	for k, v := range c.nodes {
		nodes[k] = v
	}
	c.mu.RUnlock()

	for mapID, ownerID := range owners {
		owner := nodes[ownerID]
		if owner == nil || !owner.IsHealthy() {
			continue
		}
		cp, err := owner.Checkpoint(mapID)
		if err != nil {
			continue
		}
		if err := c.store.SaveCheckpoint(cp); err != nil {
			return err
		}
		replicaID := replicas[mapID]
		if replica, ok := nodes[replicaID]; ok {
			replica.StoreReplica(cp)
		}
	}
	return nil
}

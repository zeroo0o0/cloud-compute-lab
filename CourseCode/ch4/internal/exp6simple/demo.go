package exp6simple

import "fmt"

type StorageDemo struct {
	dbPlayers map[string]int
	dbConfigs map[string]string
	cache     map[string]string
}

func NewStorageDemo() *StorageDemo {
	return &StorageDemo{
		dbPlayers: map[string]int{
			"player_1": 100,
		},
		dbConfigs: map[string]string{
			"drop_rate": "1.5",
		},
		cache: map[string]string{
			"gold:player_1": "100",
		},
	}
}

func PrintRunHints() {
	fmt.Println("说明：")
	fmt.Println("- 这是实验六的简化版，不需要 Docker、Redis、PostgreSQL。")
	fmt.Println("- 程序内部用两个 map（内存字典）分别扮演：持久层（DB）和缓存层（Cache）。")
	fmt.Println()
}

func (s *StorageDemo) DeductGold(userID string, deductAmount int) error {
	fmt.Printf("[Write Through] 开始扣除 %s 金币 %d...\n", userID, deductAmount)

	currentGold, ok := s.dbPlayers[userID]
	if !ok {
		return fmt.Errorf("玩家 %s 不存在", userID)
	}
	currentGold -= deductAmount

	/*
		================ 【学生重点 实验六简化版：Write Through】 ================
		这里不连真实数据库，但保留同样的分层顺序：
		1. 先写“持久层” map。
		2. 再把新值同步到“缓存层” map。

		重点不是 map 本身，而是“核心资产先落到底层，再同步上层”。
		==================================================================
	*/
	s.dbPlayers[userID] = currentGold
	fmt.Printf("[Write Through] 持久层已更新：players[%s]=%d\n", userID, currentGold)

	s.cache["gold:"+userID] = fmt.Sprintf("%d", currentGold)
	fmt.Printf("[Write Through] 缓存层已同步：cache[gold:%s]=%d\n\n", userID, currentGold)
	return nil
}

func (s *StorageDemo) ShowGoldConsistency(userID string) {
	dbGold, ok := s.dbPlayers[userID]
	if !ok {
		fmt.Printf("[一致性检查] 持久层中不存在玩家 %s\n", userID)
		return
	}

	cacheGold, ok := s.cache["gold:"+userID]
	if !ok {
		cacheGold = "<未命中>"
	}

	fmt.Printf("[状态检查] 持久层.players[%s]=%d | 缓存层.cache[gold:%s]=%s\n\n", userID, dbGold, userID, cacheGold)
}

func (s *StorageDemo) GetGameConfig(key string) string {
	cacheKey := "cfg:" + key

	/*
		================ 【学生重点 实验六简化版：Cache Aside 读路径】 ================
		配置类数据是“读多写少”，所以先查缓存层：
		1. 缓存命中：直接返回。
		2. 缓存未命中：去持久层读。
		3. 读到后写回缓存，供下一次命中。
		=====================================================================
	*/
	if val, ok := s.cache[cacheKey]; ok {
		fmt.Printf("[Cache Aside 读] 缓存命中 %s=%s\n\n", key, val)
		return val
	}

	fmt.Println("[Cache Aside 读] 缓存未命中，开始查询持久层...")

	dbVal, ok := s.dbConfigs[key]
	if !ok {
		fmt.Printf("[Cache Aside 读] 持久层中不存在配置 %s\n\n", key)
		return ""
	}

	s.cache[cacheKey] = dbVal
	fmt.Printf("[Cache Aside 读] 已从持久层读取 %s=%s，并回填到缓存层\n\n", key, dbVal)
	return dbVal
}

func (s *StorageDemo) UpdateGameConfig(key, newVal string) {
	fmt.Printf("[Cache Aside 写] 开始更新 %s=%s ...\n", key, newVal)

	/*
		================ 【学生重点 实验六简化版：Cache Aside 写路径】 ================
		写配置时保留原实验的关键策略：
		1. 先写持久层。
		2. 再删除缓存层里的旧值。

		删除后，下一次读取会 miss，再从持久层读新值并回填。
		=====================================================================
	*/
	s.dbConfigs[key] = newVal
	fmt.Printf("[Cache Aside 写] 持久层已更新：configs[%s]=%s\n", key, newVal)

	delete(s.cache, "cfg:"+key)
	fmt.Printf("[Cache Aside 写] 缓存层旧值已删除：cache[cfg:%s]\n\n", key)
}

func (s *StorageDemo) ShowConfigState(key string) {
	dbVal, ok := s.dbConfigs[key]
	if !ok {
		fmt.Printf("[状态检查] 持久层中不存在配置 %s\n", key)
		return
	}

	cacheVal, ok := s.cache["cfg:"+key]
	if !ok {
		cacheVal = "<未命中>"
	}

	fmt.Printf("[状态检查] 持久层.configs[%s]=%s | 缓存层.cache[cfg:%s]=%s\n\n", key, dbVal, key, cacheVal)
}

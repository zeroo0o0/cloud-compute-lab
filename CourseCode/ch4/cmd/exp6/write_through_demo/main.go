package main

import (
	"fmt"

	"ch4/internal/exp6demo"
)

func main() {
	fmt.Println("=== 实验六：分层存储架构 / Write Through ===")
	fmt.Println("目标：演示核心资产写入时，先写 PostgreSQL，再同步 Redis。")

	redisAddr := exp6demo.DefaultRedisAddr()
	pgDSN := exp6demo.DefaultPGDSN()
	exp6demo.PrintRunHints(redisAddr, pgDSN)
	if pgDSN == "" {
		fmt.Println("[错误] 未设置 PG_DSN。为了避免在代码里写死个人账号密码，实验六要求每位使用者自行配置 PostgreSQL 连接串。")
		exp6demo.PrintInfraHelp()
		return
	}

	demo, err := exp6demo.NewStorageDemo(redisAddr, pgDSN)
	if err != nil {
		fmt.Printf("[错误] 初始化基础设施失败：%v\n", err)
		exp6demo.PrintInfraHelp()
		return
	}
	defer demo.Close()

	fmt.Println("[演示前] 查看初始金币状态：")
	demo.ShowGoldConsistency("player_1")

	if err := demo.DeductGold("player_1", 20); err != nil {
		fmt.Printf("[错误] Write Through 执行失败：%v\n", err)
		return
	}

	fmt.Println("[演示后] 再次查看金币状态：")
	demo.ShowGoldConsistency("player_1")
	fmt.Println("[结论] 核心资产适合 Write Through，因为写库成功后还能立刻把缓存同步到最新值。")
}

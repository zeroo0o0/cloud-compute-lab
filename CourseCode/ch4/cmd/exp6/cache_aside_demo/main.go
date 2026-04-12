package main

import (
	"fmt"

	"ch4/internal/exp6demo"
)

func main() {
	fmt.Println("=== 实验六：分层存储架构 / Cache Aside ===")
	fmt.Println("目标：演示配置数据的缓存未命中、回填、失效与再次命中。")

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

	fmt.Println("[演示前] 查看配置在 PostgreSQL / Redis 中的状态：")
	demo.ShowConfigState("drop_rate")

	demo.GetGameConfig("drop_rate")
	demo.GetGameConfig("drop_rate")
	demo.UpdateGameConfig("drop_rate", "2.0")

	fmt.Println("[写后] 配置更新后立即查看状态：")
	demo.ShowConfigState("drop_rate")

	demo.GetGameConfig("drop_rate")

	fmt.Println("[回填后] 再次查看状态：")
	demo.ShowConfigState("drop_rate")

	demo.GetGameConfig("drop_rate")
	fmt.Println("[结论] 读多写少的配置数据适合 Cache Aside：读时回填，写时删缓存。")
}

package main

import (
	"fmt"

	"ch4/internal/exp6simple"
)

func main() {
	fmt.Println("=== 实验六：分层存储架构 / Write Through（简化版） ===")
	fmt.Println("目标：不引入真实数据库，先用内存分层演示核心资产为什么要先写底层，再同步上层。")
	fmt.Println()



	exp6simple.PrintRunHints()
	demo := exp6simple.NewStorageDemo()

	fmt.Println("---- 初始状态 ----")
	demo.ShowGoldConsistency("player_1")

	/*
		================ 【学生重点 实验六简化版：Write Through 入口】 ================
		这里只保留教学最关键的顺序：
		1. 金币属于核心资产。
		2. 修改时先更新持久层。
		3. 持久层成功后，再同步缓存层。

		先把“为什么要分层、为什么顺序不能反”讲清楚，
		再切到完整版去看真实 PostgreSQL / Redis 会更自然。
		==================================================================
	*/
	fmt.Println("---- Step 1: 扣除金币，预期先写持久层，再同步缓存 ----")
	if err := demo.DeductGold("player_1", 20); err != nil {
		fmt.Printf("[错误] Write Through 执行失败：%v\n", err)
		return
	}

	fmt.Println("---- 更新后状态 ----")
	demo.ShowGoldConsistency("player_1")

}

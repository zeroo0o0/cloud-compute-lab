package main

import (
	"fmt"

	"ch4/internal/exp6simple"
)

func main() {
	fmt.Println("=== 实验六：分层存储架构 / Cache Aside（简化版） ===")
	fmt.Println("目标：不引入真实数据库，先用内存分层演示配置数据的 miss、回填、失效与再次命中。")
	fmt.Println()

	fmt.Println()

	exp6simple.PrintRunHints()
	demo := exp6simple.NewStorageDemo()

	fmt.Println("---- 初始状态 ----")
	demo.ShowConfigState("drop_rate")

	/*
		================ 【学生重点 实验六简化版：Cache Aside 演示顺序】 ================
		下面保留和完整版一样的“读、再读、写、再读、再读”顺序：
		1. 第一次 Get：缓存没有，去持久层读，并回填缓存层。
		2. 第二次 Get：缓存命中。
		3. Update：先写持久层，再删除缓存层旧值。
		4. 写后第一次 Get：再次 miss，从持久层读新值并回填。
		5. 最后一次 Get：再次命中缓存层。
		=====================================================================
	*/
	fmt.Println("---- Step 1: 第一次读取，预期缓存未命中 ----")
	demo.GetGameConfig("drop_rate")

	fmt.Println("---- Step 2: 第二次读取，预期直接命中缓存 ----")
	demo.GetGameConfig("drop_rate")

	fmt.Println("---- Step 3: 更新配置，预期先写持久层，再删除缓存 ----")
	demo.UpdateGameConfig("drop_rate", "2.0")

	fmt.Println("---- 写后状态 ----")
	demo.ShowConfigState("drop_rate")

	fmt.Println("---- Step 4: 更新后第一次读取，预期再次未命中并回填 ----")
	demo.GetGameConfig("drop_rate")

	fmt.Println("---- 回填后状态 ----")
	demo.ShowConfigState("drop_rate")

	fmt.Println("---- Step 5: 再次读取，预期命中缓存 ----")
	demo.GetGameConfig("drop_rate")

	fmt.Println("[结论] Cache Aside 的关键是：读时回填，写时删除旧缓存。")
}

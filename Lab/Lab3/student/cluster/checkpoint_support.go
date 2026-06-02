package cluster

import "errors"

/*
TODO C-4：地图检查点与副本同步的测试入口

这个文件不对应新的独立任务，它属于 C-4。

为什么不直接在测试里调用 checkpointLoop：
1. checkpointLoop 是后台循环，会按时间间隔一直运行。
2. 自动测试需要一个确定性的入口，立即执行一次 checkpoint。
3. RunCheckpointOnce 和 checkpointLoop 应该复用同一套逻辑：遍历主地图、抓取 checkpoint、保存到热数据、同步给副本。

你需要完成：
1. 从当前 owner 节点抓取每张主地图的 MapCheckpoint。
2. 调用 storage 保存 checkpoint。
3. 找到对应 replica 节点并同步 checkpoint。

注意：checkpointLoop 是服务端后台定时任务；RunCheckpointOnce 是测试和调试用的一次性入口。两者都属于 C-4，不是两个任务。
*/
func (c *Cluster) RunCheckpointOnce() error {
	// TODO C-4：完成一次地图检查点与副本同步。
	// 要求：从所有主地图抓 checkpoint，保存到存储，并同步给副本节点。
	return errors.New("[Lab3-C4] TODO 未实现：RunCheckpointOnce，需要完成主地图 checkpoint 与副本同步")
}

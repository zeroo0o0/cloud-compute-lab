package cluster

/*
TODO A-3：2PC 两阶段提交

这个文件负责“跨节点交易要么全部成功，要么全部失败”。

在游戏中的作用：
1. 玩家 A 和玩家 B 可能在不同地图、不同节点上。
2. TransferTreasures 会把两个地图节点包装成事务参与者 Participant。
3. Coordinator.TransferWithParticipants 负责协调 prepare / commit / abort。
4. 自动测试会通过跨节点战利品转移检查 2PC 是否真正接入游戏流程。

你需要完成：
1. 检查 txID、amount、参与者是否有效。
2. 第一阶段：先让转出方 prepare 扣减，再让转入方 prepare 增加。
3. 第二阶段：如果全部 prepare 成功，再对所有已 prepare 的参与者 commit。
4. 回滚阶段：任一 prepare 失败时，已经 prepare 的参与者必须 abort。

关键要求：
1. 不能出现 alice 被扣了战利品但 bob 没收到的情况。
2. prepare 失败后不能残留未清理的事务状态。
*/

import (
	"errors"
)

type Participant interface {
	Prepare(txID, account string, delta int) error
	Commit(txID string) error
	Abort(txID string) error
}

type Coordinator struct{}

func (c *Coordinator) Transfer(txID string, from Participant, fromAccount string, to Participant, toAccount string, amount int) error {
	return c.TransferWithParticipants(txID, from, fromAccount, to, toAccount, amount)
}

func (c *Coordinator) TransferWithParticipants(txID string, from Participant, fromAccount string, to Participant, toAccount string, amount int) error {
	return errors.New("[Lab3-A3] TODO 未实现：Coordinator.TransferWithParticipants，需要完成 prepare/commit/abort 两阶段提交")
}

package cluster

import (
	"errors"
	"fmt"
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
	if txID == "" {
		return errors.New("事务 ID 不能为空")
	}
	if amount <= 0 {
		return errors.New("转移金额必须为正数")
	}
	if from == nil || to == nil {
		return errors.New("2PC 参与者不能为空")
	}
	prepared := make([]Participant, 0, 2)
	if err := from.Prepare(txID, fromAccount, -amount); err != nil {
		return err
	}
	prepared = append(prepared, from)
	if err := to.Prepare(txID, toAccount, amount); err != nil {
		_ = from.Abort(txID)
		return err
	}
	prepared = append(prepared, to)
	for _, participant := range prepared {
		if err := participant.Commit(txID); err != nil {
			return fmt.Errorf("2PC commit 失败: %w", err)
		}
	}
	return nil
}

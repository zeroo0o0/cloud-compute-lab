package twopc

import (
	"errors"
	"fmt"
	"sync"
)

type preparedDelta struct {
	Account string
	Delta   int
}

type MemoryParticipant struct {
	mu             sync.Mutex
	ID             string
	balances       map[string]int
	prepared       map[string]preparedDelta
	failPrepareFor map[string]bool
}

func NewMemoryParticipant(id string, balances map[string]int) *MemoryParticipant {
	copyBalances := make(map[string]int, len(balances))
	for k, v := range balances {
		copyBalances[k] = v
	}
	return &MemoryParticipant{
		ID:             id,
		balances:       copyBalances,
		prepared:       make(map[string]preparedDelta),
		failPrepareFor: make(map[string]bool),
	}
}

func (p *MemoryParticipant) SetPrepareFailure(account string, shouldFail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failPrepareFor[account] = shouldFail
}

func (p *MemoryParticipant) Prepare(txID, account string, delta int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failPrepareFor[account] {
		return fmt.Errorf("参与者 %s 拒绝为账户 %s prepare", p.ID, account)
	}
	if _, exists := p.prepared[txID]; exists {
		return nil
	}
	balance := p.balances[account]
	if balance+delta < 0 {
		return fmt.Errorf("账户 %s 余额不足", account)
	}
	p.prepared[txID] = preparedDelta{Account: account, Delta: delta}
	return nil
}

func (p *MemoryParticipant) Commit(txID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pending, ok := p.prepared[txID]
	if !ok {
		return fmt.Errorf("参与者 %s 未找到事务 %s 的 prepare 记录", p.ID, txID)
	}
	p.balances[pending.Account] += pending.Delta
	delete(p.prepared, txID)
	return nil
}

func (p *MemoryParticipant) Abort(txID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.prepared, txID)
	return nil
}

func (p *MemoryParticipant) Balance(account string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.balances[account]
}

type Participant interface {
	Prepare(txID, account string, delta int) error
	Commit(txID string) error
	Abort(txID string) error
}

type Coordinator struct{}

func (c *Coordinator) Transfer(txID string, from *MemoryParticipant, fromAccount string, to *MemoryParticipant, toAccount string, amount int) error {
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
			return err
		}
	}
	return nil
}

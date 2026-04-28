// Package protocol 定义 BattleWorld 多人游戏的消息格式与连接封装。
// 在 Lab1 基础上新增：TypeBroadcast（服务器定期推送全量状态）。
package protocol

import (
	"encoding/json"
	"net"
	"sync"
)

const (
	MapWidth    = 30
	MapHeight   = 20
	InitHP      = 100
	AttackDmg   = 30
	HealAmt     = 40
	MaxPotions  = 5
	AttackRange = 2
)

// 客户端 → 服务器
const (
	TypeJoin   = "join"
	TypeMove   = "move"
	TypeAttack = "attack"
	TypeHeal   = "heal"
)

// 服务器 → 客户端
const (
	TypeInit      = "init"
	TypeBroadcast = "broadcast" // 定期广播：全量世界状态（多人新增）
	TypeEvent     = "event"
	TypeGameOver  = "gameover"
)

const (
	DirUp    = "up"
	DirDown  = "down"
	DirLeft  = "left"
	DirRight = "right"
)

// PlayerInfo 是玩家状态的可序列化快照。
type PlayerInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	HP      int    `json:"hp"`
	MaxHP   int    `json:"max_hp"`
	Potions int    `json:"potions"`
	Alive   bool   `json:"alive"`
	Kills   int    `json:"kills"` // 击杀数（多人新增）
}

// Message 是客户端与服务器通信的基本单元。
type Message struct {
	Type    string       `json:"type"`
	Dir     string       `json:"dir,omitempty"`
	Text    string       `json:"text,omitempty"`
	YourID  int          `json:"your_id,omitempty"`
	Players []PlayerInfo `json:"players,omitempty"`
	Winner  string       `json:"winner,omitempty"`
}

// Conn 将 TCP 连接与异步发送队列绑定。
type Conn struct {
	raw      net.Conn
	encoder  *json.Encoder
	decoder  *json.Decoder
	sendMu   sync.Mutex   // 保护底层写入
	sendChan chan []byte  // 异步发送队列，存放已序列化的 JSON
	closeOnce sync.Once
}

func NewConn(c net.Conn) *Conn {
	conn := &Conn{
		raw:      c,
		encoder:  json.NewEncoder(c),
		decoder:  json.NewDecoder(c),
		sendChan: make(chan []byte, 64), // 缓冲区设为 64，防止瞬时堆积
	}
	go conn.writeLoop()
	return conn
}

// writeLoop 独立协程负责实际的 IO 写入。
func (c *Conn) writeLoop() {
	for data := range c.sendChan {
		c.sendMu.Lock()
		c.raw.Write(data)
		// 注意：JSON 编码后通常需要换行符或由 Encoder 处理。
		// 由于我们将使用 Marshal，这里直接写入并手动补换行（符合 json.Encoder 行为）。
		c.raw.Write([]byte("\n"))
		c.sendMu.Unlock()
	}
}

// Send 将消息序列化并尝试放入异步队列。如果队列满则丢弃（针对实时性高的广播）。
func (c *Conn) Send(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.SendRaw(data)
}

// SendRaw 直接发送预序列化的字节流。
func (c *Conn) SendRaw(data []byte) error {
	select {
	case c.sendChan <- data:
		return nil
	default:
		// 缓冲区满，丢弃该消息（防止慢连接卡住服务器）
		return nil 
	}
}

func (c *Conn) Receive() (Message, error) {
	var msg Message
	err := c.decoder.Decode(&msg)
	return msg, err
}

func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.sendChan)
		c.raw.Close()
	})
}

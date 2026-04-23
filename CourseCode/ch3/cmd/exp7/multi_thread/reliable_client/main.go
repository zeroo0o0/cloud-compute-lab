package main

import (
	"ch3/internal/ch3net"
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/term"
)

const (
	heartbeatAction         = "heartbeat"
	clientHeartbeatInterval = 300 * time.Millisecond
	serverSilenceTimeout    = 3 * time.Second
)

type stickyDemo struct {
	step int
}

// onOff 将布尔开关转换为 ON/OFF 字符串，便于在提示信息中展示。
func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// readSingleKey 在终端原始模式下读取单个按键。
func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

// startInputReader 启动一个按键读取协程，并返回输入通道。
// 当标准输入关闭或读取失败时，通道会被关闭。
func startInputReader(buffer int) <-chan byte {
	ch := make(chan byte, buffer)
	go func() {
		defer close(ch)
		for {
			b, err := readSingleKey()
			if err != nil {
				return
			}
			ch <- b
		}
	}()
	return ch
}

// startHeartbeatSender 启动心跳发送协程，定期向服务器发送 heartbeat 输入。
// 返回 stop 函数，用于在客户端退出时停止该协程。
func startHeartbeatSender(rc *ch3net.ReliableConn, playerID int, interval time.Duration) func() {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = rc.Send(ch3proto.InputMsg{PlayerID: playerID, Action: heartbeatAction})
			case <-stopCh:
				return
			}
		}
	}()
	return func() {
		close(stopCh)
	}
}

// parsePlayerID 从命令行参数中解析玩家 ID；若未提供或非法则返回 -1。
func parsePlayerID(args []string) int {
	if len(args) > 2 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			return v
		}
	}
	return -1
}

// simulateNaiveNoFrame 模拟错误的“无帧边界”接收端：
// 将每次读取到的字节块直接当作完整 JSON 包，从而演示半包/粘包导致的解析问题。
func (d *stickyDemo) simulateNaiveNoFrame(ws ch3proto.WorldState) (ch3proto.WorldState, bool) {
	b, err := json.Marshal(ws)
	if err != nil {
		return ch3proto.WorldState{}, false
	}

	var payload []byte
	switch d.step % 3 {
	case 0:
		payload = b // 正常整包
	case 1:
		n := len(b) / 2
		if n < 1 {
			n = 1
		}
		payload = b[:n] // 半包
	default:
		payload = append(b, b...) // 粘包（两个消息拼在一起）
	}
	d.step++

	var out ch3proto.WorldState
	if err := json.Unmarshal(payload, &out); err != nil {
		return ch3proto.WorldState{}, false
	}
	return out, true
}

// main 启动可靠传输客户端：建立连接、读取键盘输入、非阻塞收包并刷新画面。
func main() {
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	playerID := parsePlayerID(os.Args)
	conn, err := net.Dial("tcp", host+":9108")
	if err != nil {
		panic(err)
	}
	rc := ch3net.NewReliableConn(conn)
	defer rc.Close()
	_ = rc.Send(ch3proto.JoinMsg{PlayerID: playerID})
	if playerID >= 0 {
		stopHeartbeat := startHeartbeatSender(rc, playerID, clientHeartbeatInterval)
		defer stopHeartbeat()
	}
	fmt.Println("=== Step7 ReliableConn 客户端 ===")
	fmt.Println("连接到", host+":9108")
	fmt.Println("特点: 使用 rc.Recv(50ms) 非阻塞收包 + 心跳机制，支持断线后按相同 playerID 重连")
	fmt.Println("输入: 直接按 w/a/s/d 移动, j 攻击, q 退出")
	fmt.Println("演示热键: t 切换本地丢帧模拟, p 切换防粘包开关, u 触发突发发送测试")
	fmt.Println("重连示例: go run ./cmd/exp7/reliable_client 127.0.0.1 0")
	if playerID < 0 {
		fmt.Println("警告: 未指定 playerID，服务器将拒绝连接。请使用 0 或 1。")
	}
	fmt.Println()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	//开启一个读取键盘输入的协程，返回一个输入通道。该通道会在标准输入关闭或读取失败时自动关闭。
	inputCh := startInputReader(8)

	// 主循环：非阻塞收包 + 非阻塞读输入 + 渲染。
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastFrame := -1
	var lastState string
	lastServerSeen := time.Now()
	simDrop := false
	tickCount := 0
	framingSafe := true
	demo := stickyDemo{}
	buildPrompt := func() string {
		return fmt.Sprintf("输入: w/a/s/d 移动, j 攻击, q 退出, t 丢帧[%s], p 防粘包[%s], u 突发发送 > ", onOff(simDrop), onOff(framingSafe))
	}
	fmt.Print(buildPrompt())

	for {
		select {
		case <-ticker.C:
			tickCount++
			if simDrop && tickCount%3 != 0 {
				// 本地演示：故意跳过大部分收包，模拟客户端丢帧。
				continue
			}

			// 1) 非阻塞收取服务器状态（50ms 超时）。
			var ws ch3proto.WorldState
			err := rc.Recv(50*time.Millisecond, &ws)
			if err == nil {
				lastServerSeen = time.Now()
				if !framingSafe {
					decoded, ok := demo.simulateNaiveNoFrame(ws)
					if !ok {
						continue
					}
					ws = decoded
				}
				stateKey := fmt.Sprintf("%v|%s", ws.Players, ws.Event)
				if ws.Frame != lastFrame && stateKey != lastState {
					fmt.Printf("\n%s\n%s", ch3render.FormatWorldState(ws, 20, 10), buildPrompt())
					lastFrame = ws.Frame
					lastState = stateKey
				}
			} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// 超时：视为丢帧，但循环继续，避免卡死。
				if time.Since(lastServerSeen) > serverSilenceTimeout {
					fmt.Println("server heartbeat timeout")
					return
				}
			} else {
				fmt.Println("server disconnected:", err)
				return
			}

		case b, ok := <-inputCh:
			if !ok {
				fmt.Println("input closed")
				return
			}
			// 与 Step5.1 保持一致：回显按下字符，便于课堂观察输入是否被读取。
			fmt.Printf("%c", b)
			// 2) 有键盘输入时，将操作发送给服务器。
			action := "idle"
			switch b {
			case 'w', 'W':
				action = "up"
			case 's', 'S':
				action = "down"
			case 'a', 'A':
				action = "left"
			case 'd', 'D':
				action = "right"
			case 'j', 'J':
				action = "attack"
			case 't', 'T':
				simDrop = !simDrop
				fmt.Printf("\n[client] drop-frame mode => %s\n%s", onOff(simDrop), buildPrompt())
				continue
			case 'p', 'P':
				framingSafe = !framingSafe
				if framingSafe {
					demo = stickyDemo{}
				}
				fmt.Printf("\n[client] framing-safe mode => %s\n%s", onOff(framingSafe), buildPrompt())
				continue
			case 'u', 'U':
				// 课堂演示：持续发送同向输入，保证画面上有可见连续位移。
				actions := []string{"right", "right", "right", "right", "attack"}
				sent := 0
				for i := 0; i < 120; i++ {
					msg := ch3proto.InputMsg{PlayerID: playerID, Action: actions[i%len(actions)]}
					if err := rc.Send(msg); err != nil {
						fmt.Printf("\n[client] burst send err: %v\n", err)
						fmt.Print(buildPrompt())
						return
					}
					sent++
					time.Sleep(8 * time.Millisecond)
				}
				fmt.Printf("\n[client] burst send done: %d messages\n", sent)
				fmt.Print(buildPrompt())
				continue
			case 'q', 'Q':
				fmt.Println("quit")
				return
			default:
				continue
			}
			msg := ch3proto.InputMsg{PlayerID: playerID, Action: action}
			if err := rc.Send(msg); err != nil {
				fmt.Println("send err:", err)
				return
			}
		}
	}
}

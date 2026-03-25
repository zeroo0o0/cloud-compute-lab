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

type stickyDemo struct {
	step int
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// simulateNaiveNoFrame mimics a buggy receiver that treats each recv chunk as a full JSON message.
// It intentionally feeds half-packet/concat-packet payloads to show sticky-packet symptoms on UI.
func (d *stickyDemo) simulateNaiveNoFrame(ws ch3proto.WorldState) (ch3proto.WorldState, bool) {
	b, err := json.Marshal(ws)
	if err != nil {
		return ch3proto.WorldState{}, false
	}

	var payload []byte
	switch d.step % 3 {
	case 0:
		payload = b // looks fine
	case 1:
		n := len(b) / 2
		if n < 1 {
			n = 1
		}
		payload = b[:n] // half packet
	default:
		payload = append(b, b...) // sticky packet (two messages glued)
	}
	d.step++

	var out ch3proto.WorldState
	if err := json.Unmarshal(payload, &out); err != nil {
		return ch3proto.WorldState{}, false
	}
	return out, true
}

func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

func parsePlayerID(args []string) int {
	if len(args) > 2 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			return v
		}
	}
	return -1
}

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
	fmt.Println("=== Step7 ReliableConn 客户端 ===")
	fmt.Println("连接到", host+":9108")
	fmt.Println("特点: 使用 rc.Recv(50ms) 非阻塞收包，支持断线后按相同 playerID 重连")
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

	inputCh := make(chan byte, 8)
	go func() {
		for {
			b, err := readSingleKey()
			if err != nil {
				close(inputCh)
				return
			}
			inputCh <- b
		}
	}()

	// 主循环：非阻塞收包 + 非阻塞读输入 + 渲染
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastFrame := -1
	var lastState string
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
				// 本地演示用：故意跳过大部分收包，模拟客户端丢帧。
				continue
			}

			// 1. 非阻塞收取服务器状态 (50ms 超时)
			var ws ch3proto.WorldState
			err := rc.Recv(50*time.Millisecond, &ws)
			if err == nil {
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
				// 超时：丢帧但循环继续，不卡死
			} else {
				fmt.Println("server disconnected:", err)
				return
			}

		case b, ok := <-inputCh:
			if !ok {
				fmt.Println("input closed")
				return
			}
			// 与 Step5.1 一致：直接回显按下的字符，便于课堂观察输入被读取。
			fmt.Printf("%c", b)
			// 2. 有键盘输入，发给服务器
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
				// 课堂演示用：持续发送同向输入，保证画面上有可见连续位移。
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

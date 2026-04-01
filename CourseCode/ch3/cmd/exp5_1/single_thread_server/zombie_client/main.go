package main

import (
	"bufio"
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	sendTick      = 200 * time.Millisecond
	recvTimeout   = 700 * time.Millisecond
	renderEvery   = 2
	defaultHost   = "127.0.0.1"
	defaultAction = "idle"
	clearScreen   = "\033[2J\033[H"
)

type cfg struct {
	host         string
	role         int
	blackholeAt  int
	blackholeDur int
}

func main() {
	c := parseArgs(os.Args)

	conn, err := net.Dial("tcp", c.host+":9107")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("=== zombie 单线程客户端(断网演示) ===")
	fmt.Println("连接到", c.host+":9107")
	fmt.Printf("role=%d, blackholeAt=%d, blackholeDur=%d\n", c.role, c.blackholeAt, c.blackholeDur)
	fmt.Println("输入 t + 回车 可随时切换断网模拟")

	// 初始渲染：服务端要等两位玩家都连接才会下发首帧，因此这里不能设置短超时。
	fmt.Println("等待另一名玩家连接后开始...")
	var initWS ch3proto.WorldState
	if err := ch3proto.RecvJSON(conn, &initWS); err != nil {
		fmt.Println("recv init err:", err)
		return
	}
	renderFrame(initWS, 0, false, "connected")
	fmt.Println("tick sync mode: driven by server frame")

	inputCh := make(chan string, 8)
	go readInputLoop(inputCh)

	lastFrame := initWS.Frame
	lastRenderKey := stateKey(initWS)
	tickNo := 0
	blackhole := false
	timeoutCount := 0
	lastStatus := "running"
	pendingAction := defaultAction
	lastSentFrame := lastFrame
	for {
		tickNo++

		select {
		case line := <-inputCh:
			action, toggled, ok := parseInput(line)
			if !ok {
				lastStatus = "unknown input: use w/a/s/d/j or t"
				break
			}
			if toggled {
				blackhole = !blackhole
				if blackhole {
					lastStatus = "simulate disconnect: ON (no send/recv)"
				} else {
					lastStatus = "simulate disconnect: OFF (resume)"
				}
				break
			}
			pendingAction = action
			lastStatus = "queued action: " + action
		default:
		}

		if c.blackholeAt > 0 && tickNo == c.blackholeAt {
			blackhole = true
			lastStatus = "simulate disconnect: ON (scheduled)"
		}
		if c.blackholeDur > 0 && c.blackholeAt > 0 && tickNo == c.blackholeAt+c.blackholeDur {
			blackhole = false
			lastStatus = "simulate disconnect: OFF (scheduled resume)"
		}

		if blackhole {
			time.Sleep(sendTick)
			continue
		}

		nextFrame := lastFrame + 1
		if nextFrame > lastSentFrame {
			action := pendingAction
			pendingAction = defaultAction
			if err := ch3proto.SendJSON(conn, ch3proto.InputMsg{Frame: nextFrame, Action: action}); err != nil {
				fmt.Println("send err:", err)
				return
			}
			lastSentFrame = nextFrame
		}

		_ = conn.SetReadDeadline(time.Now().Add(recvTimeout))
		var ws ch3proto.WorldState
		if err := ch3proto.RecvJSON(conn, &ws); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				timeoutCount++
				if timeoutCount%3 == 0 {
					lastStatus = fmt.Sprintf("tick=%d recv timeout (server likely blocked waiting another player)", tickNo)
				}
				continue
			}
			fmt.Println("recv err:", err)
			return
		}
		_ = conn.SetReadDeadline(time.Time{})
		timeoutCount = 0

		if ws.Frame <= lastFrame {
			continue
		}
		lastFrame = ws.Frame
		if lastSentFrame < lastFrame {
			lastSentFrame = lastFrame
		}
		initWS = ws
		if ws.Frame%renderEvery == 0 {
			k := stateKey(ws)
			if k != lastRenderKey {
				renderFrame(ws, tickNo, blackhole, lastStatus)
				lastRenderKey = k
			}
		}
	}
}

func readInputLoop(inputCh chan string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			continue
		}
		select {
		case inputCh <- line:
		default:
		}
	}
}

func renderFrame(ws ch3proto.WorldState, tickNo int, blackhole bool, status string) {
	mode := "online"
	if blackhole {
		mode = "blackhole"
	}
	fmt.Print(clearScreen)
	fmt.Println(ch3render.FormatWorldState(ws, 20, 10))
	fmt.Printf("tick=%d mode=%s\n", tickNo, mode)
	fmt.Println("cmd: 输入 w/a/s/d/j + 回车 发动作, 输入 t + 回车 切换断网")
	if status != "" {
		fmt.Println("status:", status)
	}
}

func parseInput(line string) (action string, toggled bool, ok bool) {
	switch line {
	case "t":
		return "", true, true
	case "w":
		return "up", false, true
	case "a":
		return "left", false, true
	case "s":
		return "down", false, true
	case "d":
		return "right", false, true
	case "j":
		return "attack", false, true
	default:
		return "", false, false
	}
}

func parseArgs(args []string) cfg {
	c := cfg{
		host:         defaultHost,
		role:         0,
		blackholeAt:  -1,
		blackholeDur: 0,
	}
	if len(args) > 1 {
		c.host = args[1]
	}
	if len(args) > 2 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			c.role = v
		}
	}
	if len(args) > 3 {
		if v, err := strconv.Atoi(args[3]); err == nil {
			c.blackholeAt = v
		}
	}
	if len(args) > 4 {
		if v, err := strconv.Atoi(args[4]); err == nil {
			c.blackholeDur = v
		}
	}
	return c
}

func stateKey(ws ch3proto.WorldState) string {
	if len(ws.Players) < 2 {
		return fmt.Sprintf("f=%d|p=%v|e=%s", ws.Frame, ws.Players, ws.Event)
	}
	p0, p1 := ws.Players[0], ws.Players[1]
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d|%s", p0.X, p0.Y, p0.HP, p1.X, p1.Y, p1.HP, ws.Event)
}

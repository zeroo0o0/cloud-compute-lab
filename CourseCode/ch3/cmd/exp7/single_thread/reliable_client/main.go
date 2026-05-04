package main

import (
	"bufio"
	"ch3/internal/ch3net"
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}

	conn, rc := dialServer(host)
	defer conn.Close()
	fmt.Println("=== reliable 单线程客户端(断网演示) ===")
	fmt.Println("连接到", host+":9108")

	reader := bufio.NewReader(os.Stdin)
	disconnected := false
	stickyMode := false
	lastFrame := -1
	for {
		fmt.Print("输入: w/a/s/d 移动, j攻击, q退出, t断网, p粘包演示 | action> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("read input err:", err)
			return
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "t" {
			disconnected = !disconnected
			if disconnected {
				fmt.Println("simulate disconnect: ON (no send/recv)")
			} else {
				fmt.Println("simulate disconnect: OFF (resume)")
			}
			continue
		}
		if line == "p" {
			stickyMode = !stickyMode
			if stickyMode {
				_ = rc.Send(ch3proto.InputMsg{Action: "p_on"})
				fmt.Println("sticky demo: ON (raw read, may see 粘包)")
			} else {
				_ = rc.Send(ch3proto.InputMsg{Action: "p_off"})
				_ = conn.Close()
				conn, rc = dialServer(host)
				lastFrame = -1
				fmt.Println("sticky demo: OFF (reconnected, use RecvJSON framing)")
			}
			continue
		}
		if disconnected {
			fmt.Println("[disconnected] ignoring input, server will wait for next input")
			continue
		}

		action := parseAction(line)
		if action == "" {
			action = "idle"
		}
		if action == "quit" {
			_ = ch3proto.SendJSON(conn, ch3proto.InputMsg{Action: "quit"})
			fmt.Println("quit")
			return
		}
		if err := rc.Send(ch3proto.InputMsg{Action: action}); err != nil {
			fmt.Println("send err:", err)
			return
		}
		var latest ch3proto.WorldState
		if stickyMode {
			ws, err := recvRawFrame(conn, 2*time.Second)
			if err != nil {
				fmt.Printf("粘包演示: %v\n", err)
				continue
			}
			latest = ws
		} else {
			ws, ok := recvLatestAfter(rc, lastFrame, 2*time.Second)
			if !ok {
				fmt.Println("recv err: no frame received")
				return
			}
			latest = ws
		}
		lastFrame = latest.Frame
		fmt.Print(ch3render.FormatWorldState(latest, 20, 10))
		fmt.Print("\r\n")
	}
}

func dialServer(host string) (net.Conn, *ch3net.ReliableConn) {
	conn, err := net.Dial("tcp", host+":9108")
	if err != nil {
		panic(err)
	}
	return conn, ch3net.NewReliableConn(conn)
}

func recvLatestAfter(rc *ch3net.ReliableConn, lastFrame int, firstTimeout time.Duration) (ch3proto.WorldState, bool) {
	/*
		================ 【学生重点 第三章：只渲染最新帧】 ================
		网络抖动时，客户端可能一次收到多帧历史状态。
		这里先等到比 lastFrame 新的帧，再用短超时继续“追帧”，
		最终只把最新状态交给渲染，避免旧帧刷屏拖慢画面。
		============================================================
	*/
	var latest ch3proto.WorldState
	deadline := time.Now().Add(firstTimeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ch3proto.WorldState{}, false
		}
		if err := rc.Recv(remaining, &latest); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return ch3proto.WorldState{}, false
		}
		if latest.Frame > lastFrame {
			break
		}
	}
	for {
		var next ch3proto.WorldState
		err := rc.Recv(10*time.Millisecond, &next)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			return ch3proto.WorldState{}, false
		}
		if next.Frame > latest.Frame {
			latest = next
		}
	}
	return latest, true
}

func recvRawFrame(conn net.Conn, timeout time.Duration) (ch3proto.WorldState, error) {
	/*
		================ 【学生重点 第三章：故意关闭拆包】 ================
		粘包演示模式下，客户端直接 Read 原始字节并尝试当作一条 JSON 解析。
		服务端连续写多条 JSON 时，这里很容易读到连体数据，从而复现错误。
		正常模式应使用 ReliableConn/RecvJSON 的长度前缀协议。
		============================================================
	*/
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return ch3proto.WorldState{}, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return ch3proto.WorldState{}, err
	}
	var ws ch3proto.WorldState
	data := buf[:n]
	if err := json.Unmarshal(data, &ws); err != nil {
		return ch3proto.WorldState{}, fmt.Errorf("raw read failed (likely粘包): %w", err)
	}
	return ws, nil
}

func parseAction(line string) string {
	line = strings.TrimSpace(strings.ToLower(line))
	switch line {
	case "w":
		return "up"
	case "s":
		return "down"
	case "a":
		return "left"
	case "d":
		return "right"
	case "j", "attack":
		return "attack"
	case "q", "quit":
		return "quit"
	case "", "idle":
		return "idle"
	default:
		return ""
	}
}

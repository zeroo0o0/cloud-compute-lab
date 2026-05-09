package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const defaultAddr = "127.0.0.1:9103"

func main() {
	addr := flag.String("addr", defaultAddr, "server address")
	player := flag.String("player", "fast", "fast or slow")
	delayMS := flag.Int("delay-ms", -1, "input delay in milliseconds")
	flag.Parse()

	delay := defaultDelay(*player, *delayMS)
	if err := run(*addr, *player, delay); err != nil {
		fmt.Fprintln(os.Stderr, "client error:", err)
		os.Exit(1)
	}
}

func run(addr, player string, delay time.Duration) error {
	if player != "fast" && player != "slow" {
		return fmt.Errorf("-player must be fast or slow")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := writeLine(writer, "HELLO "+player); err != nil {
		return err
	}

	fmt.Printf("%s client connected, hero=%s, delay=%s\n", player, heroName(player), delay)
	for {
		line, err := readLine(reader)
		if err != nil {
			return err
		}
		switch line {
		case "START_ACTION":
			/*
				================ 【学生辅助】制造事件到达差异 ================

				这里不是事件驱动核心逻辑，只是用 delay 模拟两个英雄的事件到达时间：
				- fast：疾风英雄，20ms 后发出移动事件。
				- slow：断流英雄，500ms 后发出移动事件。

				如果你想理解“有事件/没事件怎么处理”，主要看 06_network_event_driven_warzone/server。
				============================================================
			*/
			time.Sleep(delay)
			action, dx, dy := scriptedAction(player)
			if err := writeLine(writer, fmt.Sprintf("ACTION %s %s %d %d %d", player, action, dx, dy, delay.Milliseconds())); err != nil {
				return err
			}
			fmt.Printf("%s sent ACTION %s after %s\n", heroName(player), action, delay)
		default:
			if strings.HasPrefix(line, "DONE") {
				fmt.Println("server finished:", strings.TrimSpace(strings.TrimPrefix(line, "DONE")))
				return nil
			}
		}
	}
}

func scriptedAction(player string) (string, int, int) {
	if player == "slow" {
		return "MOVE_LEFT", -1, 0
	}
	return "MOVE_RIGHT", 1, 0
}

func heroName(player string) string {
	if player == "slow" {
		return "断流骑士 slow"
	}
	return "疾风游侠 fast"
}

func defaultDelay(player string, override int) time.Duration {
	if override >= 0 {
		return time.Duration(override) * time.Millisecond
	}
	if player == "slow" {
		return 500 * time.Millisecond
	}
	return 20 * time.Millisecond
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func writeLine(writer *bufio.Writer, line string) error {
	if _, err := writer.WriteString(line + "\n"); err != nil {
		return err
	}
	return writer.Flush()
}

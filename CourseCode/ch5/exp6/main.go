package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

type actorView struct {
	Name  string
	Role  string
	State string
	Note  string
}

type step struct {
	Event   string
	Coord   actorView
	Workers []actorView
}

type scene struct {
	Key   string
	Title string
	Intro []string
	Steps []step
}

func main() {
	scenarioKey := flag.String("scenario", "all", "场景：normal | a | b | c | d | all")
	flag.Parse()

	scenes := buildScenes()
	if *scenarioKey == "all" {
		order := []string{"normal", "a", "b", "c", "d"}
		for i, key := range order {
			sc := scenes[key]
			if i > 0 {
				waitForEnter("\n--- 按 Enter 继续，进入下一场景 ---")
			}
			runScene(sc)
		}
		return
	}

	sc, ok := scenes[*scenarioKey]
	if !ok {
		fmt.Printf("未知场景: %s\n", *scenarioKey)
		os.Exit(1)
	}
	runScene(sc)
}

func runScene(sc scene) {
	fmt.Println("============================================================")
	fmt.Printf("场景：%s (%s)\n", sc.Title, sc.Key)
	for _, line := range sc.Intro {
		fmt.Println(line)
	}
	for i, st := range sc.Steps {
		waitForEnter(fmt.Sprintf("按 Enter 继续（步骤 %d/%d）...", i+1, len(sc.Steps)))
		renderStep(sc.Title, st, i+1, len(sc.Steps))
	}
}

func buildScenes() map[string]scene {
	return map[string]scene{
		"normal": {
			Key:   "normal",
			Title: "正常流程：2PC 成功提交",
			Intro: []string{
				"结构说明：上方为协调者服务，下方三个为参与者数据库。",
				"按 Enter 推进一步，查看当前状态与事件播报。",
			},
			Steps: []step{
				{
					Event: "初始化：所有参与者处于 INIT，等待协调者发起投票。",
					Coord: actorView{Name: "协调者", Role: "Service", State: "INIT", Note: "准备发起投票"},
					Workers: []actorView{
						{Name: "数据库A", Role: "Worker", State: "INIT", Note: "待命"},
						{Name: "数据库B", Role: "Worker", State: "INIT", Note: "待命"},
						{Name: "数据库C", Role: "Worker", State: "INIT", Note: "待命"},
					},
				},
				{
					Event: "协调者发送 VOTE-REQ，进入 WAIT。",
					Coord: actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "等待投票"},
					Workers: []actorView{
						{Name: "数据库A", Role: "Worker", State: "INIT", Note: "准备投票"},
						{Name: "数据库B", Role: "Worker", State: "INIT", Note: "准备投票"},
						{Name: "数据库C", Role: "Worker", State: "INIT", Note: "准备投票"},
					},
				},
				{
					Event: "参与者全部投票 YES，进入 READY。",
					Coord: actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "收到全部 YES"},
					Workers: []actorView{
						{Name: "数据库A", Role: "Worker", State: "READY", Note: "VOTE-YES"},
						{Name: "数据库B", Role: "Worker", State: "READY", Note: "VOTE-YES"},
						{Name: "数据库C", Role: "Worker", State: "READY", Note: "VOTE-YES"},
					},
				},
				{
					Event: "协调者写入全局决议 COMMIT。",
					Coord: actorView{Name: "协调者", Role: "Service", State: "COMMIT", Note: "写入决议"},
					Workers: []actorView{
						{Name: "数据库A", Role: "Worker", State: "READY", Note: "等待指令"},
						{Name: "数据库B", Role: "Worker", State: "READY", Note: "等待指令"},
						{Name: "数据库C", Role: "Worker", State: "READY", Note: "等待指令"},
					},
				},
				{
					Event: "协调者广播 GLOBAL-COMMIT，所有参与者提交完成。",
					Coord: actorView{Name: "协调者", Role: "Service", State: "COMMIT", Note: "广播完成"},
					Workers: []actorView{
						{Name: "数据库A", Role: "Worker", State: "COMMIT", Note: "提交成功"},
						{Name: "数据库B", Role: "Worker", State: "COMMIT", Note: "提交成功"},
						{Name: "数据库C", Role: "Worker", State: "COMMIT", Note: "提交成功"},
					},
				},
			},
		},
		"a": {
			Key:   "a",
			Title: "故障A：参与者拒票",
			Intro: []string{"某参与者拒绝提交，导致全局 ABORT。"},
			Steps: []step{
				{
					Event:   "初始化：等待投票。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "INIT", Note: "准备发起投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "待命"}},
				},
				{
					Event:   "协调者发送 VOTE-REQ。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "等待投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "准备投票"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "准备投票"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "准备投票"}},
				},
				{
					Event:   "数据库B 返回 NO，拒绝提交。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "收到 NO"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "READY", Note: "VOTE-YES"}, {Name: "数据库B", Role: "Worker", State: "ABORT", Note: "VOTE-NO"}, {Name: "数据库C", Role: "Worker", State: "READY", Note: "VOTE-YES"}},
				},
				{
					Event:   "协调者广播 GLOBAL-ABORT，所有参与者回滚。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "ABORT", Note: "广播回滚"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "ABORT", Note: "回滚"}, {Name: "数据库B", Role: "Worker", State: "ABORT", Note: "回滚"}, {Name: "数据库C", Role: "Worker", State: "ABORT", Note: "回滚"}},
				},
			},
		},
		"b": {
			Key:   "b",
			Title: "故障B：参与者超时无响应",
			Intro: []string{"某参与者长时间沉默，协调者等待超时后回滚。"},
			Steps: []step{
				{
					Event:   "初始化：等待投票。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "INIT", Note: "准备发起投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "待命"}},
				},
				{
					Event:   "协调者发送 VOTE-REQ。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "等待投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "READY", Note: "VOTE-YES"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "无响应"}, {Name: "数据库C", Role: "Worker", State: "READY", Note: "VOTE-YES"}},
				},
				{
					Event:   "等待超时，协调者决定 GLOBAL-ABORT。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "ABORT", Note: "超时回滚"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "ABORT", Note: "回滚"}, {Name: "数据库B", Role: "Worker", State: "ABORT", Note: "超时回滚"}, {Name: "数据库C", Role: "Worker", State: "ABORT", Note: "回滚"}},
				},
			},
		},
		"c": {
			Key:   "c",
			Title: "故障C：协调者第一阶段崩溃",
			Intro: []string{"协调者在发送投票请求前宕机，参与者超时自回滚。"},
			Steps: []step{
				{
					Event:   "初始化：等待协调者发起投票。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "INIT", Note: "准备发起投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "待命"}},
				},
				{
					Event:   "协调者宕机，投票请求未发出。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "DOWN", Note: "崩溃"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "等待"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "等待"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "等待"}},
				},
				{
					Event:   "参与者等待超时，自行 ABORT。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "DOWN", Note: "仍未恢复"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "ABORT", Note: "超时"}, {Name: "数据库B", Role: "Worker", State: "ABORT", Note: "超时"}, {Name: "数据库C", Role: "Worker", State: "ABORT", Note: "超时"}},
				},
			},
		},
		"d": {
			Key:   "d",
			Title: "故障D：决议写入后崩溃",
			Intro: []string{"协调者已写入 COMMIT 决议，但广播前崩溃，恢复后重放。"},
			Steps: []step{
				{
					Event:   "初始化：等待投票。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "INIT", Note: "准备发起投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库B", Role: "Worker", State: "INIT", Note: "待命"}, {Name: "数据库C", Role: "Worker", State: "INIT", Note: "待命"}},
				},
				{
					Event:   "协调者发送 VOTE-REQ，参与者返回 YES。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "WAIT", Note: "收集投票"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "READY", Note: "VOTE-YES"}, {Name: "数据库B", Role: "Worker", State: "READY", Note: "VOTE-YES"}, {Name: "数据库C", Role: "Worker", State: "READY", Note: "VOTE-YES"}},
				},
				{
					Event:   "协调者写入 COMMIT 决议后宕机，尚未广播。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "DOWN", Note: "决议已写盘"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "READY", Note: "等待指令"}, {Name: "数据库B", Role: "Worker", State: "READY", Note: "等待指令"}, {Name: "数据库C", Role: "Worker", State: "READY", Note: "等待指令"}},
				},
				{
					Event:   "协调者恢复，读取日志并重放 GLOBAL-COMMIT。",
					Coord:   actorView{Name: "协调者", Role: "Service", State: "COMMIT", Note: "重放决议"},
					Workers: []actorView{{Name: "数据库A", Role: "Worker", State: "COMMIT", Note: "提交完成"}, {Name: "数据库B", Role: "Worker", State: "COMMIT", Note: "提交完成"}, {Name: "数据库C", Role: "Worker", State: "COMMIT", Note: "提交完成"}},
				},
			},
		},
	}
}

func renderStep(title string, st step, index, total int) {
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("%s | 步骤 %d/%d\n", title, index, total)
	fmt.Printf("事件：%s\n\n", st.Event)
	renderLayout(st.Coord, st.Workers)
}

func renderLayout(coord actorView, workers []actorView) {
	if len(workers) != 3 {
		fmt.Println("[渲染错误] 需要 3 个参与者")
		return
	}
	coordBox := makeBox(coord, 32)
	for _, line := range coordBox {
		fmt.Println(line)
	}
	fmt.Println()
	workerBoxes := [][]string{
		makeBox(workers[0], 24),
		makeBox(workers[1], 24),
		makeBox(workers[2], 24),
	}
	for i := 0; i < len(workerBoxes[0]); i++ {
		fmt.Printf("%s  %s  %s\n", workerBoxes[0][i], workerBoxes[1][i], workerBoxes[2][i])
	}
	fmt.Println()
}

func makeBox(actor actorView, width int) []string {
	contentWidth := width - 2
	lines := []string{
		actor.Name,
		fmt.Sprintf("Role: %s", actor.Role),
		fmt.Sprintf("State: %s", actor.State),
		fmt.Sprintf("Note: %s", actor.Note),
	}
	for i, line := range lines {
		lines[i] = padRight(line, contentWidth)
	}
	box := []string{
		"+" + strings.Repeat("-", contentWidth) + "+",
	}
	for _, line := range lines {
		box = append(box, "|"+line+"|")
	}
	box = append(box, "+"+strings.Repeat("-", contentWidth)+"+")
	return box
}

func padRight(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func waitForEnter(prompt string) {
	fmt.Println(prompt)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

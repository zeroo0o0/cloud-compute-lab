package utils

import (
	"fmt"
	"strings"
	"time"
)

type CinematicLine struct {
	Speaker string
	Text    string
}

var visualStepDelay = 650 * time.Millisecond

func SetVisualStepDelay(ms int) {
	if ms <= 0 {
		visualStepDelay = 0
		return
	}
	visualStepDelay = time.Duration(ms) * time.Millisecond
}

func avatarOf(speaker string) string {
	s := strings.TrimSpace(speaker)
	switch s {
	case "教授":
		return "🧠"
	case "东京/丹佛", "东京", "丹佛":
		return "🚪"
	case "里约":
		return "💻"
	case "柏林/内罗毕", "柏林", "内罗毕":
		return "🖨️"
	case "警督":
		return "🕵️"
	case "旁白":
		return "🎙️"
	default:
		return "👤"
	}
}

func calcTypeDelay() time.Duration {
	if visualStepDelay <= 0 {
		return 0
	}
	ms := int(visualStepDelay.Milliseconds() / 40)
	if ms < 8 {
		ms = 8
	}
	if ms > 28 {
		ms = 28
	}
	return time.Duration(ms) * time.Millisecond
}

func typewrite(text string) {
	d := calcTypeDelay()
	if d <= 0 {
		fmt.Println(text)
		return
	}
	for _, r := range text {
		fmt.Printf("%c", r)
		time.Sleep(d)
	}
	fmt.Println()
}

func RenderCinematicScene(title string, background []string, lines []CinematicLine) {
	fmt.Println("============================================================")
	typewrite("🎬 " + title)
	fmt.Println()

	for _, b := range background {
		typewrite("[旁白] " + b)
	}
	if len(background) > 0 {
		fmt.Println()
	}

	for _, l := range lines {
		fmt.Printf("%s %s\n", avatarOf(l.Speaker), l.Speaker)
		typewrite("  " + l.Text)
		fmt.Println()
		if visualStepDelay > 0 {
			time.Sleep(visualStepDelay)
		}
	}
}

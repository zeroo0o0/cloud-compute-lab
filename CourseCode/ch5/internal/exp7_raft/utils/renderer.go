package utils

import (
	"fmt"
	"time"
)

var visualStepDelay = 350 * time.Millisecond

func SetVisualStepDelay(ms int) {
	if ms <= 0 {
		visualStepDelay = 0
		return
	}
	visualStepDelay = time.Duration(ms) * time.Millisecond
}

func RenderTitle(title string) {
	fmt.Println("============================================================")
	fmt.Println("🎬 " + title)
	fmt.Println()
}

func RenderLine(line string) {
	fmt.Println(line)
	if visualStepDelay > 0 {
		time.Sleep(visualStepDelay)
	}
}

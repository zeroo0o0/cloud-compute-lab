package ch3render

import (
	"fmt"
	"strings"
	"warzone/ch3/internal/ch3proto"
)

func RenderDuelMap(p0x, p0y, p1x, p1y int, width, height int) string {
	var b strings.Builder
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteByte('-')
	}
	b.WriteString("+\n")
	for y := 0; y <= height; y++ {
		b.WriteByte('|')
		for x := 0; x <= width; x++ {
			ch := '.'
			if p0x == x && p0y == y {
				ch = 'A'
			}
			if p1x == x && p1y == y {
				if ch == 'A' {
					ch = 'X'
				} else {
					ch = 'B'
				}
			}
			b.WriteByte(byte(ch))
		}
		b.WriteString("|\n")
	}
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteByte('-')
	}
	b.WriteString("+")
	return b.String()
}

func RenderWorldMap(players []ch3proto.PlayerState, width, height int) string {
	var b strings.Builder
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteByte('-')
	}
	b.WriteString("+\n")
	for y := 0; y <= height; y++ {
		b.WriteByte('|')
		for x := 0; x <= width; x++ {
			ch := '.'
			for _, p := range players {
				if !p.Online {
					continue
				}
				if p.X == x && p.Y == y {
					mark := rune('0' + p.ID)
					if ch != '.' {
						ch = 'X'
					} else {
						ch = mark
					}
				}
			}
			b.WriteRune(ch)
		}
		b.WriteString("|\n")
	}
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteByte('-')
	}
	b.WriteString("+")
	return b.String()
}

func FormatWorldState(ws ch3proto.WorldState, width, height int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[frame=%d] ", ws.Frame))
	for _, p := range ws.Players {
		status := "offline"
		if p.Online {
			status = "online"
		}
		b.WriteString(fmt.Sprintf("P%d(%d,%d,hp=%d,%s) ", p.ID, p.X, p.Y, p.HP, status))
	}
	if ws.Event != "" {
		b.WriteString("| ")
		b.WriteString(ws.Event)
	}
	b.WriteString("\n")
	b.WriteString(RenderWorldMap(ws.Players, width, height))
	return b.String()
}

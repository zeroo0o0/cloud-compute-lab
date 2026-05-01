package ch3render

import (
	"ch5/internal/proto"
	"strings"
)

func RenderDuelMap(p0x, p0y, p1x, p1y int, width, height int) string {
	var b strings.Builder
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteString("--")
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
			b.WriteByte(' ')
		}
		b.WriteString("|\n")
	}
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteString("--")
	}
	b.WriteString("+")
	return b.String()
}

func RenderWorldMap(players []proto.PlayerState, width, height int) string {
	var b strings.Builder
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteString("--")
	}
	b.WriteString("+\r\n")
	for y := 0; y <= height; y++ {
		b.WriteByte('|')
		for x := 0; x <= width; x++ {
			ch := '.'
			for _, p := range players {
				if p.X == x && p.Y == y {
					if ch == '.' {
						ch = 'P'
					} else {
						ch = 'X'
					}
				}
			}
			b.WriteRune(ch)
			b.WriteByte(' ')
		}
		b.WriteString("|\r\n")
	}
	b.WriteString("+")
	for i := 0; i <= width; i++ {
		b.WriteString("--")
	}
	b.WriteString("+")
	return b.String()
}

func FormatWorldState(players []proto.PlayerState, width, height int) string {
	return RenderWorldMap(players, width, height)
}

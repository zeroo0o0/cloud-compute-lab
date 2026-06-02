package render

import (
	"exp8/game-app/internal/proto"
	"strings"
)

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

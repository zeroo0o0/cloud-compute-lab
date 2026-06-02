package ringviz

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type MigrateRange struct {
	StartKey uint32
	EndKey   uint32
}

type virtualNode struct {
	Key     uint32
	Node    string
	Replica int
}

func collectVirtualNodes(nodes []string, replicas int, hashFn func(string) uint32) []virtualNode {
	out := make([]virtualNode, 0, len(nodes)*replicas)
	for _, node := range nodes {
		for i := 0; i < replicas; i++ {
			out = append(out, virtualNode{
				Key:     hashFn(fmt.Sprintf("%s#%d", node, i)),
				Node:    node,
				Replica: i,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func angleFromKey(key uint32) float64 {
	return 2 * math.Pi * (float64(key) / float64(math.MaxUint32))
}

func ringPoint(cx, cy, radius, angle float64) (float64, float64) {
	x := cx + radius*math.Cos(angle-math.Pi/2)
	y := cy + radius*math.Sin(angle-math.Pi/2)
	return x, y
}

func keyProgress(key uint32) float64 {
	return float64(key) / float64(math.MaxUint32)
}

func drawRangeSegment(b *strings.Builder, startProgress, endProgress, cx, cy, radius float64) {
	if endProgress <= startProgress {
		return
	}
	delta := endProgress - startProgress
	steps := int(math.Ceil(delta * 220))
	if steps < 4 {
		steps = 4
	}

	points := &strings.Builder{}
	for i := 0; i <= steps; i++ {
		p := startProgress + delta*float64(i)/float64(steps)
		angle := 2 * math.Pi * p
		x, y := ringPoint(cx, cy, radius, angle)
		if i > 0 {
			points.WriteByte(' ')
		}
		fmt.Fprintf(points, "%.1f,%.1f", x, y)
	}

	fmt.Fprintf(b, `<polyline points="%s" fill="none" stroke="#dc2626" stroke-width="8" stroke-linecap="round" stroke-linejoin="round" opacity="0.9"/>`+"\n", points.String())
}

func drawMigrateRangesOnRing(b *strings.Builder, ranges []MigrateRange, cx, cy, radius float64) {
	for _, r := range ranges {
		start := keyProgress(r.StartKey)
		end := keyProgress(r.EndKey)
		if end > start {
			drawRangeSegment(b, start, end, cx, cy, radius)
			continue
		}

		// Wrap-around interval: split into [start,1] and [0,end].
		drawRangeSegment(b, start, 1.0, cx, cy, radius)
		drawRangeSegment(b, 0.0, end, cx, cy, radius)
	}
}

func rangeMidProgress(startProgress, endProgress float64) float64 {
	if endProgress >= startProgress {
		return (startProgress + endProgress) / 2
	}

	span := (1.0 - startProgress) + endProgress
	mid := startProgress + span/2
	if mid >= 1.0 {
		mid -= 1.0
	}
	return mid
}

func virtualNodeFill(node, newNode string) string {
	if node == newNode {
		return "#e76f51"
	}
	palette := []string{
		"#64748b",
		"#2a9d8f",
		"#457b9d",
		"#8a6f3d",
		"#7c6f64",
		"#6d597a",
	}
	idx := 0
	for _, ch := range node {
		idx = (idx*31 + int(ch)) % len(palette)
	}
	return palette[idx]
}

func drawVirtualLegend(b *strings.Builder, boxX, boxY float64) {
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="300" height="168" rx="18" fill="#fffdf7" stroke="#d6d3d1" stroke-width="1.5"/>`+"\n", boxX, boxY)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="22" font-family="Segoe UI" font-weight="700" fill="#1f2937">图例</text>`+"\n", boxX+20, boxY+34)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#64748b" stroke="#0f172a" stroke-width="1.3"/>`+"\n", boxX+26, boxY+66)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">原有节点的虚拟节点</text>`+"\n", boxX+48, boxY+72)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="6" fill="#e76f51" stroke="#0f172a" stroke-width="1.3"/>`+"\n", boxX+26, boxY+102)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">新增节点的虚拟节点</text>`+"\n", boxX+48, boxY+108)
	fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dc2626" stroke-width="8" stroke-linecap="round"/>`+"\n", boxX+16, boxY+138, boxX+38, boxY+138)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">迁移区间</text>`+"\n", boxX+48, boxY+144)
}
func drawVirtualStatsCard(b *strings.Builder, boxX, boxY float64, nodes []string, replicas int, totalVirtual int) {
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="300" height="160" rx="18" fill="#f8fafc" stroke="#d6d3d1" stroke-width="1.5"/>`+"\n", boxX, boxY)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="22" font-family="Segoe UI" font-weight="700" fill="#1f2937">环信息</text>`+"\n", boxX+20, boxY+34)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="18" font-family="Segoe UI" fill="#334155">真实节点数：%d</text>`+"\n", boxX+20, boxY+70, len(nodes))
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="18" font-family="Segoe UI" fill="#334155">每节点虚拟节点：%d</text>`+"\n", boxX+20, boxY+102, replicas)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="18" font-family="Segoe UI" fill="#334155">环上虚拟节点总数：%d</text>`+"\n", boxX+20, boxY+134, totalVirtual)
}

func writeRingSVG(path string, title string, nodes []string, newNode string, replicas int, highlightRanges []MigrateRange, hashFn func(string) uint32) error {
	const (
		canvasWidth  = 1440
		canvasHeight = 1200
		cx           = 560.0
		cy           = 650.0
		ringRadius   = 330.0
		sidebarX     = 1040.0
	)

	vnodes := collectVirtualNodes(nodes, replicas, hashFn)
	b := &strings.Builder{}

	fmt.Fprintln(b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", canvasWidth, canvasHeight, canvasWidth, canvasHeight)
	fmt.Fprintf(b, `<rect x="0" y="0" width="%d" height="%d" fill="#f7f4ea"/>`+"\n", canvasWidth, canvasHeight)
	fmt.Fprintf(b, `<rect x="40" y="40" width="%d" height="%d" rx="30" fill="#fffdf8" stroke="#e7e5e4" stroke-width="2"/>`+"\n", canvasWidth-80, canvasHeight-80)
	fmt.Fprintf(b, `<text x="70" y="86" font-size="34" font-family="Segoe UI" font-weight="700" fill="#111827">%s</text>`+"\n", title)
	fmt.Fprintf(b, `<text x="70" y="122" font-size="19" font-family="Segoe UI" fill="#475569">每个真实节点拆成多个虚拟节点并均匀散列到环上，玩家顺时针找到第一个虚拟节点。</text>`+"\n")
	fmt.Fprintf(b, `<text x="70" y="152" font-size="19" font-family="Segoe UI" fill="#475569">真实节点数=%d，replicas=%d，环上虚拟节点总数=%d</text>`+"\n", len(nodes), replicas, len(vnodes))

	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#334155" stroke-width="4"/>`+"\n", cx, cy, ringRadius)
	if len(highlightRanges) > 0 {
		drawMigrateRangesOnRing(b, highlightRanges, cx, cy, ringRadius)
	}

	for _, v := range vnodes {
		angle := angleFromKey(v.Key)
		x, y := ringPoint(cx, cy, ringRadius, angle)
		fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="5.8" fill="%s" stroke="#0f172a" stroke-width="1.2"/>`+"\n", x, y, virtualNodeFill(v.Node, newNode))
	}

	drawVirtualLegend(b, sidebarX, 96)
	drawVirtualStatsCard(b, sidebarX, 360, nodes, replicas, len(vnodes))

	fmt.Fprintln(b, `</svg>`)

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func WriteClassroomVisuals(initialNodes []string, newNode string, replicas int, hashFn func(string) uint32, highlightRanges []MigrateRange) error {
	imagesDir := filepath.Join("images", "virtual")
	if err := WriteVirtualBeforeVisual(imagesDir, initialNodes, replicas, hashFn); err != nil {
		return err
	}
	if err := WriteVirtualAfterVisual(imagesDir, initialNodes, newNode, replicas, hashFn, highlightRanges); err != nil {
		return err
	}
	return nil
}
func WriteVirtualBeforeVisual(imagesDir string, initialNodes []string, replicas int, hashFn func(string) uint32) error {
	beforePath := filepath.Join(imagesDir, "ring_virtual_before.svg")
	return writeRingSVG(beforePath, "一致性哈希环（虚拟节点版，扩容前）", initialNodes, "", replicas, nil, hashFn)
}

func WriteVirtualAfterVisual(imagesDir string, initialNodes []string, newNode string, replicas int, hashFn func(string) uint32, highlightRanges []MigrateRange) error {
	afterPath := filepath.Join(imagesDir, "ring_virtual_after.svg")
	afterNodes := append(append([]string{}, initialNodes...), newNode)
	return writeRingSVG(afterPath, "一致性哈希环（虚拟节点版，扩容后）", afterNodes, newNode, replicas, highlightRanges, hashFn)
}

func WriteClassroomBeforeVisual(initialNodes []string, replicas int, hashFn func(string) uint32) error {
	return WriteVirtualBeforeVisual(filepath.Join("images", "virtual"), initialNodes, replicas, hashFn)
}

func WriteClassroomAfterVisual(initialNodes []string, newNode string, replicas int, hashFn func(string) uint32, highlightRanges []MigrateRange) error {
	return WriteVirtualAfterVisual(filepath.Join("images", "virtual"), initialNodes, newNode, replicas, hashFn, highlightRanges)
}

package ringviz

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type physicalNode struct {
	Key   uint32
	Node  string
	IsNew bool
}

func collectPhysicalNodes(nodes []string, newNode string, hashFn func(string) uint32) []physicalNode {
	out := make([]physicalNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, physicalNode{
			Key:   hashFn(node),
			Node:  node,
			IsNew: node == newNode,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func formatHash(key uint32) string {
	return fmt.Sprintf("%010d", key)
}

func drawWrappedRangeSegment(b *strings.Builder, startProgress, endProgress, cx, cy, radius float64) {
	if endProgress > startProgress {
		drawRangeSegment(b, startProgress, endProgress, cx, cy, radius)
		return
	}
	drawRangeSegment(b, startProgress, 1.0, cx, cy, radius)
	drawRangeSegment(b, 0.0, endProgress, cx, cy, radius)
}

func drawPhysicalLegend(b *strings.Builder) {
	boxX := 1110.0
	boxY := 96.0
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="290" height="212" rx="18" fill="#fffdf7" stroke="#d6d3d1" stroke-width="1.5"/>`+"\n", boxX, boxY)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="22" font-family="Segoe UI" font-weight="700" fill="#1f2937">图例</text>`+"\n", boxX+20, boxY+34)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="11" fill="#64748b" stroke="#0f172a" stroke-width="2"/>`+"\n", boxX+26, boxY+66)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">原有节点</text>`+"\n", boxX+48, boxY+72)
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="11" fill="#e76f51" stroke="#0f172a" stroke-width="2"/>`+"\n", boxX+26, boxY+102)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">新增节点</text>`+"\n", boxX+48, boxY+108)
	fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dc2626" stroke-width="8" stroke-linecap="round"/>`+"\n", boxX+16, boxY+138, boxX+38, boxY+138)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">迁移区间</text>`+"\n", boxX+48, boxY+144)
	fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-width="2.5" stroke-dasharray="6 6"/>`+"\n", boxX+16, boxY+174, boxX+38, boxY+174)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="17" font-family="Segoe UI" fill="#334155">标注引线</text>`+"\n", boxX+48, boxY+180)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="16" font-family="Segoe UI" fill="#64748b">环内文字表示顺时针方向的区间归属。</text>`+"\n", boxX+20, boxY+202)
}

func drawOwnershipNotes(b *strings.Builder, nodes []physicalNode, cx, cy, radius float64) {
	for i, node := range nodes {
		prev := nodes[(i-1+len(nodes))%len(nodes)]
		mid := rangeMidProgress(keyProgress(prev.Key), keyProgress(node.Key))
		angle := 2 * math.Pi * mid
		x, y := ringPoint(cx, cy, radius, angle)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="13" font-family="Segoe UI" fill="#64748b" text-anchor="middle">归属 %s</text>`+"\n", x, y, node.Node)
	}
}

func physicalNodeFill(node physicalNode) string {
	if node.IsNew {
		return "#e76f51"
	}
	return "#64748b"
}

func writePhysicalRingSVG(path, title string, nodes []string, newNode string, highlightRanges []MigrateRange, hashFn func(string) uint32) error {
	const (
		canvasWidth  = 1440
		canvasHeight = 1400
		cx           = 560.0
		cy           = 700.0
		ringRadius   = 330.0
		labelRadius  = 425.0
	)

	pnodes := collectPhysicalNodes(nodes, newNode, hashFn)
	b := &strings.Builder{}

	fmt.Fprintln(b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", canvasWidth, canvasHeight, canvasWidth, canvasHeight)
	fmt.Fprintf(b, `<rect x="0" y="0" width="%d" height="%d" fill="#f7f4ea"/>`+"\n", canvasWidth, canvasHeight)
	// Title and ring now share one visual panel, so the ring feels like part of the same explanation.
	fmt.Fprintf(b, `<rect x="40" y="40" width="%d" height="%d" rx="30" fill="#fffdf8" stroke="#e7e5e4" stroke-width="2"/>`+"\n", canvasWidth-80, canvasHeight-80)
	fmt.Fprintf(b, `<text x="70" y="86" font-size="34" font-family="Segoe UI" font-weight="700" fill="#111827">%s</text>`+"\n", title)
	fmt.Fprintf(b, `<text x="70" y="122" font-size="19" font-family="Segoe UI" fill="#475569">每个物理节点在环上只占一个位置，并负责从前驱节点到自己之间的顺时针区间。</text>`+"\n")
	fmt.Fprintf(b, `<text x="70" y="152" font-size="19" font-family="Segoe UI" fill="#475569">物理节点数=%d，环上节点数=%d</text>`+"\n", len(nodes), len(pnodes))

	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#334155" stroke-width="4"/>`+"\n", cx, cy, ringRadius)
	if len(highlightRanges) > 0 {
		for _, r := range highlightRanges {
			drawWrappedRangeSegment(b, keyProgress(r.StartKey), keyProgress(r.EndKey), cx, cy, ringRadius)
		}
	}

	drawOwnershipNotes(b, pnodes, cx, cy, ringRadius-56)

	for _, node := range pnodes {
		angle := angleFromKey(node.Key)
		x, y := ringPoint(cx, cy, ringRadius, angle)
		lx, ly := ringPoint(cx, cy, labelRadius, angle)
		color := physicalNodeFill(node)

		anchor := "start"
		textX := lx + 10
		boxX := textX - 8
		boxY := ly - 24
		textTitleY := ly - 5
		textHashY := ly + 14

		if lx < cx {
			anchor = "end"
			textX = lx - 10
			boxX = textX - 120
		}

		// Keep top-side labels away from the page title area.
		if ly < 220 {
			if lx < cx {
				textX = lx - 36
				boxX = textX - 120
			} else {
				textX = lx + 36
				boxX = textX - 8
			}
			boxY = 172
			textTitleY = boxY + 19
			textHashY = boxY + 38
		}

		fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#94a3b8" stroke-width="2" stroke-dasharray="6 6"/>`+"\n", x, y, lx, ly)
		fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="14" fill="%s" stroke="#0f172a" stroke-width="2.5"/>`+"\n", x, y, color)
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="128" height="44" rx="10" fill="#fffdf8" stroke="#d6d3d1" stroke-width="1.5"/>`+"\n", boxX, boxY)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="16" font-family="Segoe UI" font-weight="700" fill="#111827" text-anchor="%s">%s</text>`+"\n", textX, textTitleY, anchor, node.Node)
		fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-size="12" font-family="Consolas" fill="#475569" text-anchor="%s">hash=%s</text>`+"\n", textX, textHashY, anchor, formatHash(node.Key))
	}

	if len(highlightRanges) > 0 {
		r := highlightRanges[0]
		fmt.Fprintf(b, `<rect x="70" y="1200" width="1280" height="110" rx="18" fill="#fff7ed" stroke="#fdba74" stroke-width="1.5"/>`+"\n")
		fmt.Fprintf(b, `<text x="95" y="1235" font-size="24" font-family="Segoe UI" font-weight="700" fill="#9a3412">迁移说明</text>`+"\n")
		fmt.Fprintf(b, `<text x="95" y="1265" font-size="18" font-family="Segoe UI" fill="#7c2d12">新增节点接管红色弧段，对应区间为 (%s, %s]。</text>`+"\n", formatHash(r.StartKey), formatHash(r.EndKey))
		fmt.Fprintf(b, `<text x="95" y="1292" font-size="18" font-family="Segoe UI" fill="#7c2d12">也就是从前驱节点开始，顺时针走到新节点为止的那一段键空间。</text>`+"\n")
	}

	drawPhysicalLegend(b)
	fmt.Fprintln(b, `</svg>`)

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func WritePhysicalBeforeVisual(imagesDir string, initialNodes []string, hashFn func(string) uint32) error {
	beforePath := filepath.Join(imagesDir, "ring_physical_before.svg")
	return writePhysicalRingSVG(beforePath, "一致性哈希环（物理节点版，扩容前）", initialNodes, "", nil, hashFn)
}

func WritePhysicalAfterVisual(imagesDir string, initialNodes []string, newNode string, hashFn func(string) uint32, highlightRanges []MigrateRange) error {
	afterPath := filepath.Join(imagesDir, "ring_physical_after.svg")
	afterNodes := append(append([]string{}, initialNodes...), newNode)
	return writePhysicalRingSVG(afterPath, "一致性哈希环（物理节点版，扩容后）", afterNodes, newNode, highlightRanges, hashFn)
}

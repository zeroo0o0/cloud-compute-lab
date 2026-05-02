package main

import (
	"bufio"
	"crypto/md5"
	"encoding/binary"
	"exp3/internal/ringviz"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type HashRing struct {
	ring   map[uint32]string
	sorted []uint32
}

// MigrateRange 表示扩容后被新节点接管的一段哈希空间，区间语义为 (StartKey, EndKey]。
type MigrateRange struct {
	From     string
	To       string
	StartKey uint32
	EndKey   uint32
}

func hash(s string) uint32 {
	sum := md5.Sum([]byte(s))
	return binary.BigEndian.Uint32(sum[:4])
}

// BuildRing 构建“物理节点直接上环”的哈希环：每个物理节点只对应环上的一个 hash 位置。
func BuildRing(nodes []string) *HashRing {
	r := &HashRing{
		ring: make(map[uint32]string, len(nodes)),
	}
	for _, node := range nodes {
		AddNode(r, node)
	}
	return r
}

// AddNode 把物理节点放到环上，并维护有序 hash 列表，方便后续顺时针查找。
func AddNode(r *HashRing, node string) {
	key := hash(node)
	r.ring[key] = node
	r.sorted = append(r.sorted, key)
	sort.Slice(r.sorted, func(i, j int) bool {
		return r.sorted[i] < r.sorted[j]
	})
}

// Lookup 体现一致性哈希的核心规则：玩家 hash 后，顺时针找到第一个节点；越过末尾则回到环首。
func Lookup(r *HashRing, playerID string) string {
	if len(r.sorted) == 0 {
		return ""
	}
	h := hash(playerID)
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= h
	})
	if idx == len(r.sorted) {
		idx = 0
	}
	return r.ring[r.sorted[idx]]
}

// MigrateRanges 计算新增物理节点后需要迁移的区间。
// 物理节点直接上环时，新节点只会接管“前驱节点 -> 新节点”这一段连续空间。
func MigrateRanges(r *HashRing, newNode string) []MigrateRange {
	if len(r.sorted) == 0 {
		return nil
	}

	newKey := hash(newNode)
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= newKey
	})

	prevIdx := len(r.sorted) - 1
	if idx > 0 {
		prevIdx = idx - 1
	}

	srcIdx := 0
	if idx < len(r.sorted) {
		srcIdx = idx
	}

	return []MigrateRange{
		{
			From:     r.ring[r.sorted[srcIdx]],
			To:       newNode,
			StartKey: r.sorted[prevIdx],
			EndKey:   newKey,
		},
	}
}

func buildNodeNames(n int) []string {
	nodes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, fmt.Sprintf("Node-%d", i))
	}
	return nodes
}

// distribute 用大量玩家样本模拟 key 分布，用来观察当前哈希环是否负载均衡。
func distribute(r *HashRing, playerCount int) map[string]int {
	stats := make(map[string]int)
	for i := 0; i < playerCount; i++ {
		id := fmt.Sprintf("player-%06d", i)
		stats[Lookup(r, id)]++
	}
	return stats
}

func printDistribution(title string, stats map[string]int, total int) {
	fmt.Println(title)
	nodes := make([]string, 0, len(stats))
	for node := range stats {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	max := 0
	min := total
	for _, node := range nodes {
		c := stats[node]
		if c > max {
			max = c
		}
		if c < min {
			min = c
		}
		fmt.Printf("  %-7s -> %6d (%.2f%%)\n", node, c, float64(c)*100/float64(total))
	}

	avg := float64(total) / float64(len(nodes))
	fmt.Printf("  max=%d, min=%d, avg=%.2f\n\n", max, min, avg)
}

// buildAssignments 记录扩容前/后的玩家归属，用于计算一致性哈希扩容时真正发生迁移的比例。
func buildAssignments(r *HashRing, playerCount int) map[string]string {
	m := make(map[string]string, playerCount)
	for i := 0; i < playerCount; i++ {
		id := fmt.Sprintf("player-%06d", i)
		m[id] = Lookup(r, id)
	}
	return m
}

// remapRatio 对比扩容前后的玩家归属，展示一致性哈希“只迁移局部 key”的特点。
func remapRatio(before, after map[string]string) (int, float64) {
	changed := 0
	for id, oldNode := range before {
		if after[id] != oldNode {
			changed++
		}
	}
	return changed, float64(changed) * 100 / float64(len(before))
}

func readInt(scanner *bufio.Scanner, prompt string, fallback int, min int) int {
	for {
		fmt.Print(prompt)
		if !scanner.Scan() {
			return fallback
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return fallback
		}

		value, err := strconv.Atoi(input)
		if err != nil || value < min {
			fmt.Printf("请输入不小于 %d 的整数\n", min)
			continue
		}
		return value
	}
}

func waitForEnter(scanner *bufio.Scanner, prompt string) {
	fmt.Print(prompt)
	scanner.Scan()
}

func resetImagesDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	initialNodes := readInt(scanner, "请输入初始节点数量（默认 4）: ", 4, 1)
	players := readInt(scanner, "请输入用户数量（默认 100000）: ", 100000, 1)
	newNode := fmt.Sprintf("Node-%d", initialNodes)
	imagesDir := filepath.Join("images", "physical")

	nodes := buildNodeNames(initialNodes)
	ringBefore := BuildRing(nodes)
	beforeStats := distribute(ringBefore, players)
	beforeAssign := buildAssignments(ringBefore, players)

	// 先基于扩容前的环计算迁移区间，这正是新增节点会接管的那段 hash 空间。
	migrate := MigrateRanges(ringBefore, newNode)
	visualRanges := make([]ringviz.MigrateRange, 0, len(migrate))
	for _, r := range migrate {
		visualRanges = append(visualRanges, ringviz.MigrateRange{
			StartKey: r.StartKey,
			EndKey:   r.EndKey,
		})
	}

	if err := resetImagesDir(imagesDir); err != nil {
		fmt.Printf("清理 %s 文件夹失败: %v\n", filepath.Clean(imagesDir), err)
		return
	}
	if err := ringviz.WritePhysicalBeforeVisual(imagesDir, nodes, hash); err != nil {
		fmt.Printf("生成 before 可视化失败: %v\n", err)
		return
	}

	fmt.Println("=== 实验三：一致性哈希（物理节点版）===")
	fmt.Printf("初始节点=%v, 玩家数=%d\n\n", nodes, players)
	printDistribution(fmt.Sprintf("[1] 初始 %d 节点负载分布", initialNodes), beforeStats, players)
	fmt.Printf("已在 %s 生成 ring_physical_before.svg\n\n", filepath.Clean(imagesDir))

	waitForEnter(scanner, "按回车加入新节点...")
	fmt.Println()

	// 加入新节点后重新构建玩家归属，再和 beforeAssign 对比得到重映射比例。
	ringAfter := BuildRing(nodes)
	AddNode(ringAfter, newNode)
	afterAssign := buildAssignments(ringAfter, players)
	changed, ratio := remapRatio(beforeAssign, afterAssign)

	if err := ringviz.WritePhysicalAfterVisual(imagesDir, nodes, newNode, hash, visualRanges); err != nil {
		fmt.Printf("生成 after 可视化失败: %v\n", err)
		return
	}

	fmt.Println("[2] 扩容后重映射比例")
	fmt.Printf("  新增节点=%s\n", newNode)
	fmt.Printf("  发生迁移玩家=%d / %d (%.2f%%)\n\n", changed, players, ratio)

	fmt.Println("[3] MigrateRanges 迁移区间摘要")
	for i, r := range migrate {
		fmt.Printf("  %02d. %s -> %s  (%d, %d]\n", i+1, r.From, r.To, r.StartKey, r.EndKey)
	}
	fmt.Printf("\n已在 %s 生成 ring_physical_after.svg\n", filepath.Clean(imagesDir))
}

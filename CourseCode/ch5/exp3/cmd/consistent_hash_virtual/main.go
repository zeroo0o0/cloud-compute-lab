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
	ring     map[uint32]string
	sorted   []uint32
	replicas int
}

// MigrateRange 表示扩容后新节点的某个虚拟节点接管的哈希区间，区间语义为 (StartKey, EndKey]。
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

// BuildRing 构建虚拟节点哈希环：每个真实节点拆成 replicas 个虚拟节点分散到环上。
func BuildRing(nodes []string, replicas int) *HashRing {
	r := &HashRing{
		ring:     make(map[uint32]string),
		replicas: replicas,
	}
	for _, node := range nodes {
		AddNode(r, node)
	}
	return r
}

// AddNode 将一个真实节点的多个虚拟节点加入哈希环，key 形如 Node-0#0、Node-0#1。
func AddNode(r *HashRing, node string) {
	for i := 0; i < r.replicas; i++ {
		key := hash(fmt.Sprintf("%s#%d", node, i))
		r.ring[key] = node
		r.sorted = append(r.sorted, key)
	}
	sort.Slice(r.sorted, func(i, j int) bool {
		return r.sorted[i] < r.sorted[j]
	})
}

// Lookup 仍然遵循一致性哈希的顺时针查找规则，只是命中的环节点现在是“虚拟节点”。
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

// MigrateRanges 计算新增真实节点后由其每个虚拟节点接管的区间。
// 因为一个真实节点有多个虚拟节点，所以扩容时会产生多段更细碎的迁移区间。
func MigrateRanges(r *HashRing, newNode string) []MigrateRange {
	if len(r.sorted) == 0 {
		return nil
	}

	ranges := make([]MigrateRange, 0, r.replicas)
	for i := 0; i < r.replicas; i++ {
		newKey := hash(fmt.Sprintf("%s#%d", newNode, i))
		idx := sort.Search(len(r.sorted), func(j int) bool {
			return r.sorted[j] >= newKey
		})

		var prevKey uint32
		if idx == 0 {
			prevKey = r.sorted[len(r.sorted)-1]
		} else {
			prevKey = r.sorted[idx-1]
		}

		srcNode := r.ring[r.sorted[idx%len(r.sorted)]]
		ranges = append(ranges, MigrateRange{
			From:     srcNode,
			To:       newNode,
			StartKey: prevKey,
			EndKey:   newKey,
		})
	}
	return ranges
}

func buildNodeNames(n int) []string {
	nodes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, fmt.Sprintf("Node-%d", i))
	}
	return nodes
}

// distribute 用玩家样本模拟 key 分布，用来观察虚拟节点是否改善负载均衡。
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

// buildAssignments 记录每个玩家当前归属的真实节点，便于扩容前后做迁移对比。
func buildAssignments(r *HashRing, playerCount int) map[string]string {
	m := make(map[string]string, playerCount)
	for i := 0; i < playerCount; i++ {
		id := fmt.Sprintf("player-%06d", i)
		m[id] = Lookup(r, id)
	}
	return m
}

// remapRatio 统计扩容后有多少玩家被重新映射，用来体现一致性哈希降低迁移量的效果。
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
	initialNodes := readInt(scanner, "请输入初始真实节点数量（默认 4）：", 4, 1)
	replicas := readInt(scanner, "请输入每个真实节点的虚拟节点数量（默认 20）：", 20, 1)
	players := readInt(scanner, "请输入玩家数量（默认 100000）：", 100000, 1)
	newNode := fmt.Sprintf("Node-%d", initialNodes)
	imagesDir := filepath.Join("images", "virtual")

	if err := resetImagesDir(imagesDir); err != nil {
		fmt.Printf("清理 %s 文件夹失败: %v\n", filepath.Clean(imagesDir), err)
		return
	}

	nodes := buildNodeNames(initialNodes)
	ringBefore := BuildRing(nodes, replicas)
	beforeStats := distribute(ringBefore, players)
	beforeAssign := buildAssignments(ringBefore, players)

	if err := ringviz.WriteVirtualBeforeVisual(imagesDir, nodes, replicas, hash); err != nil {
		fmt.Printf("生成 ring_virtual_before.svg 失败: %v\n", err)
		return
	}

	fmt.Println("=== 实验三：一致性哈希（虚拟节点版）===")
	fmt.Printf("初始节点=%v, replicas=%d, 玩家数=%d\n\n", nodes, replicas, players)
	printDistribution(fmt.Sprintf("[1] 初始 %d 节点负载分布", initialNodes), beforeStats, players)
	fmt.Printf("已在 %s 生成 ring_virtual_before.svg\n\n", filepath.Clean(imagesDir))

	waitForEnter(scanner, "按回车加入新节点...")
	fmt.Println()

	// 迁移区间必须基于扩容前的环计算：新虚拟节点会接管其前驱到自身之间的空间。
	migrate := MigrateRanges(ringBefore, newNode)
	visualRanges := make([]ringviz.MigrateRange, 0, len(migrate))
	for _, r := range migrate {
		visualRanges = append(visualRanges, ringviz.MigrateRange{StartKey: r.StartKey, EndKey: r.EndKey})
	}

	// 加入新真实节点后，它的 replicas 个虚拟节点会一起参与后续查找。
	ringAfter := BuildRing(nodes, replicas)
	AddNode(ringAfter, newNode)
	afterAssign := buildAssignments(ringAfter, players)
	changed, ratio := remapRatio(beforeAssign, afterAssign)

	if err := ringviz.WriteVirtualAfterVisual(imagesDir, nodes, newNode, replicas, hash, visualRanges); err != nil {
		fmt.Printf("生成 ring_virtual_after.svg 失败: %v\n", err)
		return
	}

	fmt.Println("[2] 扩容后重映射比例")
	fmt.Printf("  新增节点=%s\n", newNode)
	fmt.Printf("  发生迁移玩家=%d / %d (%.2f%%)\n\n", changed, players, ratio)

	fmt.Println("[3] MigrateRanges 迁移区间摘要")
	for i, r := range migrate {
		fmt.Printf("  %02d. %s -> %s  (%d, %d]\n", i+1, r.From, r.To, r.StartKey, r.EndKey)
	}
	fmt.Printf("\n  区间条数=%d (应等于 replicas=%d)\n", len(migrate), replicas)
	fmt.Printf("已在 %s 生成 ring_virtual_after.svg\n", filepath.Clean(imagesDir))
}

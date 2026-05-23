package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type testCase struct {
	Name  string
	Title string
}

var cases = []testCase{
	{Name: "TestLab31TopologyAndConsistentHash", Title: "【测试 1】注册登录、拓扑发布与一致性哈希分片"},
	{Name: "TestLab32MapSwitchAndRouting", Title: "【测试 2】多地图切换与节点路由"},
	{Name: "TestLab33BossSharedStateAcrossMaps", Title: "【测试 3】世界 Boss 跨地图共享血量"},
	{Name: "TestLab34TreasureTransferWithTwoPhaseCommit", Title: "【测试 4】跨节点战利品转移与 2PC 回滚"},
	{Name: "TestLab35CheckpointAndColdHotPersistence", Title: "【测试 5】地图检查点与冷热数据恢复"},
	{Name: "TestLab36FailoverWithGossipAndRaft", Title: "【测试 6】Gossip 故障发现与 Raft 主从切换"},
}

func main() {
	srcDir := os.Getenv("LAB3_SRC_DIR")
	if strings.TrimSpace(srcDir) == "" {
		cwd, err := os.Getwd()
		must(err)
		srcDir = filepath.Join(filepath.Dir(cwd), "student")
	}
	cache := os.Getenv("GOCACHE")

	tempDir, err := os.MkdirTemp("", "lab3-test.")
	must(err)
	defer os.RemoveAll(tempDir)

	must(os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(fmt.Sprintf("module lab3spec\n\ngo 1.21\n\nrequire battleworld v0.0.0\n\nreplace battleworld => %s\n", filepath.Clean(srcDir))), 0o644))
	must(os.WriteFile(filepath.Join(tempDir, "lab3_game_test.go"), []byte(gameTestSource), 0o644))

	selected := selectCases(os.Args[1:])
	passed, failed := 0, 0
	for _, tc := range selected {
		fmt.Println(tc.Title)
		cmd := exec.Command("go", "test", "-run", "^"+tc.Name+"$", "-count=1", "-v")
		cmd.Dir = tempDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		cmd.Env = os.Environ()
		if cache != "" {
			cmd.Env = append(cmd.Env, "GOCACHE="+cache)
		}
		if err := cmd.Run(); err != nil {
			failed++
			fmt.Printf("  ❌ 失败  %v\n", err)
			continue
		}
		passed++
		fmt.Println("  ✅ 通过")
	}

	fmt.Printf("\n测试完成：通过 %d 项，失败 %d 项\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func selectCases(args []string) []testCase {
	if len(args) == 0 {
		return cases
	}
	lookup := make(map[string]testCase, len(cases))
	for i, tc := range cases {
		lookup[fmt.Sprintf("%d", i+1)] = tc
		lookup[tc.Name] = tc
	}
	selected := make([]testCase, 0, len(args))
	seen := make(map[string]bool)
	for _, arg := range args {
		if tc, ok := lookup[arg]; ok && !seen[tc.Name] {
			selected = append(selected, tc)
			seen[tc.Name] = true
		}
	}
	if len(selected) == 0 {
		return cases
	}
	return selected
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const gameTestSource = `
package lab3test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"battleworld/cluster"
	"battleworld/protocol"
	"battleworld/storage"
	"battleworld/world"
)

func newClusterForTest(t *testing.T) (*cluster.Cluster, *storage.Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "lab3")
	store, err := storage.NewStore(root)
	if err != nil {
		t.Fatalf("创建临时存储失败：%v", err)
	}
	c, err := cluster.NewCluster(store)
	if err != nil {
		t.Fatalf("构造集群失败：%v", err)
	}
	return c, store, root
}

func TestLab31TopologyAndConsistentHash(t *testing.T) {
	c, _, root := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	state, err := c.Login("alice", "pw")
	if err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	if state.Self.Username != "alice" {
		t.Fatalf("登录后用户名不正确：%+v", state.Self)
	}
	if len(state.Nodes) != 3 {
		t.Fatalf("状态快照中应发布 3 个逻辑节点，实际=%d", len(state.Nodes))
	}
	if len(state.Maps) != 3 {
		t.Fatalf("状态快照中应发布 3 张地图，实际=%d", len(state.Maps))
	}

	briefs := make(map[string]protocol.MapBrief)
	for _, brief := range state.Maps {
		briefs[brief.ID] = brief
	}
	ownerSet := make(map[string]bool)
	for _, mapID := range []string{"green", "cave", "ruins"} {
		owner, replica, err := c.MapPlacement(mapID)
		if err != nil {
			t.Fatalf("查询地图 %s 的一致性哈希归属失败：%v", mapID, err)
		}
		if owner == "" || replica == "" || owner == replica {
			t.Fatalf("地图 %s 的主副本归属不正确：owner=%q replica=%q", mapID, owner, replica)
		}
		if briefs[mapID].NodeID != owner {
			t.Fatalf("地图 %s 的展示节点应来自一致性哈希 owner，展示=%s owner=%s", mapID, briefs[mapID].NodeID, owner)
		}
		ownerSet[owner] = true
	}
	if len(ownerSet) < 2 {
		t.Fatalf("三张地图不应全部集中在同一个节点，实际 owner 数=%d", len(ownerSet))
	}

	hot := loadHotSessions(t, root)
	if _, ok := hot["alice"]; !ok {
		t.Fatalf("登录后应写入在线热会话")
	}
}

func TestLab32MapSwitchAndRouting(t *testing.T) {
	c, _, _ := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	if _, err := c.Login("alice", "pw"); err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	before, err := c.SnapshotFor("alice")
	if err != nil {
		t.Fatalf("获取切图前快照失败：%v", err)
	}
	if _, err := c.SwitchMap("alice", "cave"); err != nil {
		t.Fatalf("切换地图到 cave 失败：%v", err)
	}
	after, err := c.SnapshotFor("alice")
	if err != nil {
		t.Fatalf("获取切图后快照失败：%v", err)
	}
	if before.Self.MapID == after.Self.MapID {
		t.Fatalf("切图后地图应发生变化，切图前=%s 切图后=%s", before.Self.MapID, after.Self.MapID)
	}
	owner, _, err := c.MapPlacement("cave")
	if err != nil {
		t.Fatalf("查询 cave owner 失败：%v", err)
	}
	if after.Map.NodeID != owner {
		t.Fatalf("切图后的会话路由应指向 cave 的 owner，实际=%s 期望=%s", after.Map.NodeID, owner)
	}
}

func TestLab33BossSharedStateAcrossMaps(t *testing.T) {
	c, store, _ := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	if err := c.Register("bob", "pw", "pw"); err != nil {
		t.Fatalf("注册 bob 失败：%v", err)
	}
	aliceSite := bossSiteForTest(t, "green")
	bobSite := bossSiteForTest(t, "cave")
	aliceProfile, _ := store.LoadProfile("alice")
	aliceProfile.LastMap = "green"
	aliceProfile.X = aliceSite.X
	aliceProfile.Y = aliceSite.Y
	aliceProfile.Attack = 30
	if err := store.SaveProfile(*aliceProfile); err != nil {
		t.Fatalf("设置 alice 到 green Boss 附近失败：%v", err)
	}
	bobProfile, _ := store.LoadProfile("bob")
	bobProfile.LastMap = "cave"
	bobProfile.X = bobSite.X
	bobProfile.Y = bobSite.Y
	bobProfile.Attack = 40
	if err := store.SaveProfile(*bobProfile); err != nil {
		t.Fatalf("设置 bob 到 cave Boss 附近失败：%v", err)
	}
	if _, err := c.Login("alice", "pw"); err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	if _, err := c.Login("bob", "pw"); err != nil {
		t.Fatalf("登录 bob 失败：%v", err)
	}
	alice, err := c.SnapshotFor("alice")
	if err != nil {
		t.Fatalf("获取 alice 快照失败：%v", err)
	}
	bob, err := c.SnapshotFor("bob")
	if err != nil {
		t.Fatalf("获取 bob 快照失败：%v", err)
	}
	if alice.Map.ID != "green" || bob.Map.ID != "cave" {
		t.Fatalf("测试需要两个玩家位于不同地图，alice=%s bob=%s", alice.Map.ID, bob.Map.ID)
	}
	beforeHP := alice.Boss.HP
	if _, err := c.AttackBoss("alice"); err != nil {
		t.Fatalf("alice 攻击世界 Boss 失败：%v", err)
	}
	if _, err := c.AttackBoss("bob"); err != nil {
		t.Fatalf("bob 跨地图攻击世界 Boss 失败：%v", err)
	}
	alice, _ = c.SnapshotFor("alice")
	bob, _ = c.SnapshotFor("bob")
	if alice.Boss.HP != beforeHP-70 {
		t.Fatalf("两个地图玩家攻击后 Boss 扣血不正确，攻击前=%d 攻击后=%d", beforeHP, alice.Boss.HP)
	}
	if alice.Boss.HP != bob.Boss.HP {
		t.Fatalf("Boss 扣血后仍应全服一致，alice=%d bob=%d", alice.Boss.HP, bob.Boss.HP)
	}
}

func TestLab34TreasureTransferWithTwoPhaseCommit(t *testing.T) {
	c, store, _ := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	if err := c.Register("bob", "pw", "pw"); err != nil {
		t.Fatalf("注册 bob 失败：%v", err)
	}
	aliceProfile, _ := store.LoadProfile("alice")
	aliceProfile.LastMap = "green"
	aliceProfile.Treasures = 3
	if err := store.SaveProfile(*aliceProfile); err != nil {
		t.Fatalf("更新 alice 初始战利品失败：%v", err)
	}
	bobProfile, _ := store.LoadProfile("bob")
	bobProfile.LastMap = "cave"
	bobProfile.Treasures = 0
	if err := store.SaveProfile(*bobProfile); err != nil {
		t.Fatalf("更新 bob 初始战利品失败：%v", err)
	}
	if _, err := c.Login("alice", "pw"); err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	if _, err := c.Login("bob", "pw"); err != nil {
		t.Fatalf("登录 bob 失败：%v", err)
	}
	aliceBefore, _ := c.SnapshotFor("alice")
	bobBefore, _ := c.SnapshotFor("bob")
	if aliceBefore.Map.NodeID == bobBefore.Map.NodeID {
		t.Fatalf("测试需要跨节点交易，alice 节点=%s bob 节点=%s", aliceBefore.Map.NodeID, bobBefore.Map.NodeID)
	}
	if err := c.TransferTreasures("alice", "bob", 1); err != nil {
		t.Fatalf("跨节点战利品转移失败：%v", err)
	}
	aliceAfter, _ := c.SnapshotFor("alice")
	bobAfter, _ := c.SnapshotFor("bob")
	if aliceAfter.Self.Treasures != aliceBefore.Self.Treasures-1 || bobAfter.Self.Treasures != bobBefore.Self.Treasures+1 {
		t.Fatalf("2PC 提交后的战利品结果不正确，alice=%d bob=%d", aliceAfter.Self.Treasures, bobAfter.Self.Treasures)
	}
	if err := c.TransferTreasures("alice", "bob", 99); err == nil {
		t.Fatalf("战利品不足时 2PC 应拒绝提交")
	}
	aliceRollback, _ := c.SnapshotFor("alice")
	bobRollback, _ := c.SnapshotFor("bob")
	if aliceRollback.Self.Treasures != aliceAfter.Self.Treasures || bobRollback.Self.Treasures != bobAfter.Self.Treasures {
		t.Fatalf("2PC prepare 失败后必须保持双方状态不变，alice=%d bob=%d", aliceRollback.Self.Treasures, bobRollback.Self.Treasures)
	}
}

func TestLab35CheckpointAndColdHotPersistence(t *testing.T) {
	c, _, root := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	if _, err := c.Login("alice", "pw"); err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	if _, err := c.SwitchMap("alice", "cave"); err != nil {
		t.Fatalf("切换到 cave 失败：%v", err)
	}
	checkpointPath := filepath.Join(root, "data", "hot", "checkpoints", "cave.json")
	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("清理旧 checkpoint 失败：%v", err)
	}
	if err := c.RunCheckpointOnce(); err != nil {
		t.Fatalf("执行一次 checkpoint 失败：%v", err)
	}
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("checkpoint 文件未生成：%v", err)
	}
	beforeLogout, _ := c.SnapshotFor("alice")
	if err := c.Logout("alice"); err != nil {
		t.Fatalf("退出登录失败：%v", err)
	}
	relogin, err := c.Login("alice", "pw")
	if err != nil {
		t.Fatalf("重新登录失败：%v", err)
	}
	if relogin.Map.ID != "cave" || relogin.Self.X != beforeLogout.Self.X || relogin.Self.Y != beforeLogout.Self.Y {
		t.Fatalf("冷数据未恢复退出前地图和坐标，退出前=%s(%d,%d) 重登=%s(%d,%d)", beforeLogout.Map.ID, beforeLogout.Self.X, beforeLogout.Self.Y, relogin.Map.ID, relogin.Self.X, relogin.Self.Y)
	}
}

func TestLab36FailoverWithGossipAndRaft(t *testing.T) {
	c, _, _ := newClusterForTest(t)
	if err := c.Register("alice", "pw", "pw"); err != nil {
		t.Fatalf("注册 alice 失败：%v", err)
	}
	if _, err := c.Login("alice", "pw"); err != nil {
		t.Fatalf("登录 alice 失败：%v", err)
	}
	ownerBefore, replicaBefore, err := c.MapPlacement("green")
	if err != nil {
		t.Fatalf("查询 green 布局失败：%v", err)
	}
	if replicaBefore == "" {
		t.Fatalf("green 应存在副本节点")
	}
	logBefore := c.MetadataLogLength()
	text, err := c.ExecuteAdmin("故障", ownerBefore)
	if err != nil {
		t.Fatalf("触发故障失败：%v", err)
	}
	if !strings.Contains(text, "故障") {
		t.Fatalf("管理命令应返回故障提示，实际=%s", text)
	}
	ownerAfter, _, err := c.MapPlacement("green")
	if err != nil {
		t.Fatalf("查询故障后布局失败：%v", err)
	}
	if ownerAfter != replicaBefore {
		t.Fatalf("故障切换后应由原副本接管，实际=%s 期望=%s", ownerAfter, replicaBefore)
	}
	status, err := c.MemberStatus(ownerBefore)
	if err != nil {
		t.Fatalf("查询故障节点 gossip 状态失败：%v", err)
	}
	if status != string(cluster.Dead) {
		t.Fatalf("故障节点应被 Gossip 成员表标记为 dead，实际=%s", status)
	}
	if c.MetadataLeader() == "" {
		t.Fatalf("故障切换需要 Raft 元数据 leader")
	}
	if c.MetadataLogLength() <= logBefore {
		t.Fatalf("故障切换应通过 Raft 提交新的地图 owner 元数据，提交前日志=%d 提交后=%d", logBefore, c.MetadataLogLength())
	}
	after, err := c.SnapshotFor("alice")
	if err != nil {
		t.Fatalf("故障后获取玩家快照失败：%v", err)
	}
	if after.Map.NodeID != ownerAfter {
		t.Fatalf("故障后玩家会话路由应修正到新主节点，实际=%s 期望=%s", after.Map.NodeID, ownerAfter)
	}
}

func loadHotSessions(t *testing.T, root string) map[string]protocol.HotSession {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "data", "hot", "sessions.json"))
	if err != nil {
		t.Fatalf("读取热会话文件失败：%v；通常说明 persistSessionState 没有写入 HotSession", err)
	}
	hot := make(map[string]protocol.HotSession)
	if err := json.Unmarshal(data, &hot); err != nil {
		t.Fatalf("解析热会话文件失败：%v", err)
	}
	return hot
}

func bossSiteForTest(t *testing.T, mapID string) protocol.BossSite {
	t.Helper()
	for _, cfg := range world.AvailableMaps() {
		if cfg.ID == mapID {
			return protocol.BossSite{MapID: mapID, X: cfg.BossX, Y: cfg.BossY}
		}
	}
	t.Fatalf("未找到地图 %s", mapID)
	return protocol.BossSite{}
}
`

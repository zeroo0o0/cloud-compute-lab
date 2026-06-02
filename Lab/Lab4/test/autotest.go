// Lab4 Kubernetes 自动测试程序
//
// 用法：
//
//	go run test/autotest.go
//	go run test/autotest.go 1 3
//
// 环境变量：
//
//	LAB4_NAMESPACE=lab4
//	LAB4_GATEWAY_ADDR=120.79.8.174:30910
//	LAB4_KUBECTL=kubectl
//	LAB4_SKIP_DISRUPTIVE=1   跳过删除 Pod 的恢复测试
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	typeRegister   = "register"
	typeLogin      = "login"
	typeQuickEnter = "quick_enter"
	typeLogout     = "logout"
	typeMove       = "move"
	typeSwitchMap  = "switch_map"
	typeAdmin      = "admin"
	typeAuth       = "auth"
	typeState      = "state"
	typeError      = "error"

	dirRight = "right"
	dirDown  = "down"
)

type testCase struct {
	ID          string
	Title       string
	Disruptive  bool
	Run         func(*tester) error
	Description string
}

type tester struct {
	namespace  string
	addr       string
	kubectlBin string
	prefix     string
}

type message struct {
	Type     string      `json:"type"`
	Action   string      `json:"action,omitempty"`
	Username string      `json:"username,omitempty"`
	Password string      `json:"password,omitempty"`
	Dir      string      `json:"dir,omitempty"`
	MapID    string      `json:"map_id,omitempty"`
	Confirm  string      `json:"confirm,omitempty"`
	NodeID   string      `json:"node_id,omitempty"`
	Text     string      `json:"text,omitempty"`
	OK       bool        `json:"ok,omitempty"`
	Error    string      `json:"error,omitempty"`
	State    *worldState `json:"state,omitempty"`
}

type worldState struct {
	Self           playerView         `json:"self"`
	Map            mapView            `json:"map"`
	Maps           []mapBrief         `json:"maps"`
	Nodes          []nodeView         `json:"nodes"`
	OnlinePlayers  []onlinePlayerView `json:"online_players"`
	Events         []string           `json:"events"`
	SessionVersion int64              `json:"session_version"`
}

type playerView struct {
	Username  string `json:"username"`
	MapID     string `json:"map_id"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	HP        int    `json:"hp"`
	Treasures int    `json:"treasures"`
}

type mapView struct {
	ID      string       `json:"id"`
	NodeID  string       `json:"node_id"`
	Players []playerView `json:"players"`
	Version int64        `json:"version"`
}

type mapBrief struct {
	ID      string `json:"id"`
	NodeID  string `json:"node_id"`
	Players int    `json:"players"`
	Version int64  `json:"version"`
}

type nodeView struct {
	ID      string `json:"id"`
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
}

type onlinePlayerView struct {
	Username string `json:"username"`
	MapID    string `json:"map_id"`
	NodeID   string `json:"node_id"`
}

type gameConn struct {
	raw *net.TCPConn
	enc *json.Encoder
	dec *json.Decoder
}

var cases = []testCase{
	{ID: "1", Title: "【测试 1】Kubernetes 资源、Service、HPA、Redis 检查", Run: testKubernetesResources},
	{ID: "2", Title: "【测试 2】游戏入口、登录、移动、切图、Redis 状态检查", Run: testGameProtocolAndRedis},
	{ID: "3", Title: "【测试 3】Pod 回收保护注解与 deletion-cost 检查", Run: testPodLifecycleAnnotations},
	{ID: "4", Title: "【测试 4】删除 gateway Pod 后自动恢复", Disruptive: true, Run: testGatewayRecovery},
	{ID: "5", Title: "【测试 5】删除 map Pod 后 checkpoint 恢复", Disruptive: true, Run: testMapRecovery},
}

func main() {
	t := &tester{
		namespace:  env("LAB4_NAMESPACE", "lab4"),
		addr:       env("LAB4_GATEWAY_ADDR", "120.79.8.174:30910"),
		kubectlBin: env("LAB4_KUBECTL", "kubectl"),
		prefix:     fmt.Sprintf("autotest-%d", time.Now().Unix()),
	}
	selected := selectCases(os.Args[1:])
	skipDisruptive := env("LAB4_SKIP_DISRUPTIVE", "") == "1"

	fmt.Println("═══ BattleWorld Lab4 Kubernetes 自动测试 ═══")
	fmt.Printf("namespace=%s gateway=%s kubectl=%s\n\n", t.namespace, t.addr, t.kubectlBin)

	passed, failed, skipped := 0, 0, 0
	for _, tc := range selected {
		if skipDisruptive && tc.Disruptive {
			fmt.Printf("%s\n  ⏭ 跳过：LAB4_SKIP_DISRUPTIVE=1\n", tc.Title)
			skipped++
			continue
		}
		fmt.Println(tc.Title)
		start := time.Now()
		if err := tc.Run(t); err != nil {
			failed++
			fmt.Printf("  ❌ 失败：%v\n\n", err)
			continue
		}
		passed++
		fmt.Printf("  ✅ 通过（%s）\n\n", time.Since(start).Truncate(time.Millisecond))
	}

	fmt.Printf("测试完成：通过 %d 项，失败 %d 项，跳过 %d 项\n", passed, failed, skipped)
	if failed > 0 {
		os.Exit(1)
	}
}

func testKubernetesResources(t *tester) error {
	requiredDeployments := []string{
		"deployment/lab4-gateway",
		"deployment/lab4-coordinator",
		"deployment/lab4-map-green",
		"deployment/lab4-map-cave",
		"deployment/lab4-map-ruins",
	}
	for _, name := range requiredDeployments {
		if err := t.kubectlOK("get", name); err != nil {
			return err
		}
		if err := t.kubectlOK("rollout", "status", name, "--timeout=180s"); err != nil {
			return err
		}
	}
	for _, name := range []string{
		"svc/lab4-gateway",
		"svc/lab4-coordinator",
		"svc/lab4-map-green",
		"svc/lab4-map-cave",
		"svc/lab4-map-ruins",
		"svc/lab4-redis",
		"statefulset/lab4-redis",
	} {
		if err := t.kubectlOK("get", name); err != nil {
			return err
		}
	}
	if err := t.kubectlOK("rollout", "status", "statefulset/lab4-redis", "--timeout=180s"); err != nil {
		return err
	}

	hpaRaw, err := t.kubectlJSON("get", "hpa", "-o", "json")
	if err != nil {
		return err
	}
	var hpaList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				MinReplicas *int32 `json:"minReplicas"`
				MaxReplicas int32  `json:"maxReplicas"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(hpaRaw, &hpaList); err != nil {
		return err
	}
	wantHPA := map[string]bool{
		"lab4-gateway":     false,
		"lab4-coordinator": false,
		"lab4-map-green":   false,
		"lab4-map-cave":    false,
		"lab4-map-ruins":   false,
	}
	for _, item := range hpaList.Items {
		if _, ok := wantHPA[item.Metadata.Name]; !ok {
			continue
		}
		wantHPA[item.Metadata.Name] = true
		if item.Spec.MinReplicas == nil || *item.Spec.MinReplicas != 1 || item.Spec.MaxReplicas != 10 {
			return fmt.Errorf("HPA %s 期望 min=1 max=10，实际 min=%v max=%d", item.Metadata.Name, item.Spec.MinReplicas, item.Spec.MaxReplicas)
		}
	}
	for name, ok := range wantHPA {
		if !ok {
			return fmt.Errorf("缺少 HPA %s", name)
		}
	}
	return nil
}

func testGameProtocolAndRedis(t *tester) error {
	username := t.prefix + "-game"
	c, state, err := dialQuick(t.addr, username, "pw")
	if err != nil {
		return err
	}
	defer c.closeWithLogout()

	if state.Self.Username != username {
		return fmt.Errorf("登录用户不匹配：got=%s want=%s", state.Self.Username, username)
	}
	if len(state.Maps) < 3 {
		return fmt.Errorf("期望至少 3 张地图，实际=%d", len(state.Maps))
	}
	if _, err := c.sendAndWait(message{Type: typeMove, Dir: dirRight}, 4*time.Second); err != nil {
		return fmt.Errorf("移动失败：%w", err)
	}
	switched, err := c.sendAndWait(message{Type: typeSwitchMap, MapID: "cave"}, 5*time.Second)
	if err != nil {
		return fmt.Errorf("切图失败：%w", err)
	}
	if switched.Self.MapID != "cave" || switched.Map.ID != "cave" {
		return fmt.Errorf("切图后应在 cave，self=%s map=%s", switched.Self.MapID, switched.Map.ID)
	}

	wait := func() error {
		keys, err := t.redisKeys("lab4*")
		if err != nil {
			return err
		}
		required := []string{
			"lab4:user:" + username,
			"lab4:session:" + username,
			"lab4:checkpoint:cave",
			"lab4-leader-coordinator",
		}
		for _, key := range required {
			if !contains(keys, key) {
				return fmt.Errorf("Redis 缺少 key %s，已有=%v", key, keys)
			}
		}
		return nil
	}
	return retry(10, time.Second, wait)
}

func testPodLifecycleAnnotations(t *tester) error {
	raw, err := t.kubectlJSON("get", "pods", "-l", "app.kubernetes.io/name=never-match", "-o", "json")
	if err == nil && len(raw) == 0 {
		return errors.New("kubectl JSON 返回为空")
	}
	pods, err := t.listPods()
	if err != nil {
		return err
	}
	for _, app := range []string{"lab4-gateway", "lab4-coordinator", "lab4-map-green", "lab4-map-cave", "lab4-map-ruins"} {
		pod := firstRunningPod(pods, app)
		if pod == nil {
			return fmt.Errorf("没有找到运行中的 Pod app=%s", app)
		}
		ann := pod.Metadata.Annotations
		if ann["controller.kubernetes.io/pod-deletion-cost"] == "" {
			return fmt.Errorf("Pod %s 缺少 pod-deletion-cost 注解", pod.Metadata.Name)
		}
		if ann["lab4/active-players"] == "" {
			return fmt.Errorf("Pod %s 缺少 lab4/active-players 注解", pod.Metadata.Name)
		}
		if ann["lab4/draining"] == "" {
			return fmt.Errorf("Pod %s 缺少 lab4/draining 注解", pod.Metadata.Name)
		}
	}
	return nil
}

func testGatewayRecovery(t *tester) error {
	username := t.prefix + "-gw"
	c, before, err := dialQuick(t.addr, username, "pw")
	if err != nil {
		return err
	}
	defer c.closeWithLogout()

	pod, err := t.firstRunningPodByApp("lab4-gateway")
	if err != nil {
		return err
	}
	if err := t.kubectlOK("delete", "pod", pod.Metadata.Name, "--wait=false"); err != nil {
		return err
	}
	if err := t.kubectlOK("rollout", "status", "deployment/lab4-gateway", "--timeout=240s"); err != nil {
		return err
	}

	reconnected, after, err := reconnectUntil(t.addr, username, "pw", 40*time.Second)
	if err != nil {
		return err
	}
	reconnected.closeWithLogout()
	if after.Self.Username != before.Self.Username {
		return fmt.Errorf("重连后用户名不一致：before=%s after=%s", before.Self.Username, after.Self.Username)
	}
	return nil
}

func testMapRecovery(t *tester) error {
	username := t.prefix + "-map"
	c, _, err := dialQuick(t.addr, username, "pw")
	if err != nil {
		return err
	}
	defer c.closeWithLogout()

	state, err := c.sendAndWait(message{Type: typeSwitchMap, MapID: "green"}, 5*time.Second)
	if err != nil {
		return err
	}
	state, err = c.sendAndWait(message{Type: typeMove, Dir: dirDown}, 5*time.Second)
	if err != nil {
		return err
	}
	beforeX, beforeY := state.Self.X, state.Self.Y

	pod, err := t.firstRunningPodByApp("lab4-map-green")
	if err != nil {
		return err
	}
	if err := t.kubectlOK("delete", "pod", pod.Metadata.Name, "--wait=false"); err != nil {
		return err
	}
	if err := t.kubectlOK("rollout", "status", "deployment/lab4-map-green", "--timeout=240s"); err != nil {
		return err
	}

	reconnected, after, err := reconnectUntil(t.addr, username, "pw", 45*time.Second)
	if err != nil {
		return err
	}
	reconnected.closeWithLogout()
	if after.Self.MapID != "green" {
		return fmt.Errorf("map 恢复后应仍在 green，实际=%s", after.Self.MapID)
	}
	if after.Self.X == 0 && after.Self.Y == 0 {
		return fmt.Errorf("map 恢复后坐标异常：(%d,%d)，删除前=(%d,%d)", after.Self.X, after.Self.Y, beforeX, beforeY)
	}
	return nil
}

func dialQuick(addr, username, password string) (*gameConn, *worldState, error) {
	raw, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	tcp, ok := raw.(*net.TCPConn)
	if !ok {
		_ = raw.Close()
		return nil, nil, errors.New("连接不是 TCPConn")
	}
	c := &gameConn{raw: tcp, enc: json.NewEncoder(tcp), dec: json.NewDecoder(bufio.NewReader(tcp))}
	if err := c.send(message{Type: typeQuickEnter, Username: username, Password: password, Confirm: password}); err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	msg, err := c.recvWithin(6 * time.Second)
	if err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	if msg.Type == typeError || !msg.OK || msg.State == nil {
		_ = c.raw.Close()
		return nil, nil, fmt.Errorf("认证失败：%s", msg.Error)
	}
	return c, msg.State, nil
}

func reconnectUntil(addr, username, password string, timeout time.Duration) (*gameConn, *worldState, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		c, state, err := dialLogin(addr, username, password)
		if err == nil {
			return c, state, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return nil, nil, fmt.Errorf("重连超时：%w", lastErr)
}

func dialLogin(addr, username, password string) (*gameConn, *worldState, error) {
	raw, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	tcp := raw.(*net.TCPConn)
	c := &gameConn{raw: tcp, enc: json.NewEncoder(tcp), dec: json.NewDecoder(bufio.NewReader(tcp))}
	if err := c.send(message{Type: typeLogin, Username: username, Password: password}); err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	msg, err := c.recvWithin(6 * time.Second)
	if err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	if msg.Type == typeError || !msg.OK || msg.State == nil {
		_ = c.raw.Close()
		return nil, nil, fmt.Errorf("登录失败：%s", msg.Error)
	}
	return c, msg.State, nil
}

func (c *gameConn) send(m message) error {
	return c.enc.Encode(m)
}

func (c *gameConn) recvWithin(timeout time.Duration) (message, error) {
	var msg message
	_ = c.raw.SetReadDeadline(time.Now().Add(timeout))
	err := c.dec.Decode(&msg)
	_ = c.raw.SetReadDeadline(time.Time{})
	return msg, err
}

func (c *gameConn) sendAndWait(m message, timeout time.Duration) (*worldState, error) {
	if err := c.send(m); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := c.recvWithin(time.Until(deadline))
		if err != nil {
			return nil, err
		}
		if msg.Type == typeError {
			return nil, errors.New(msg.Error)
		}
		if msg.State != nil {
			return msg.State, nil
		}
	}
	return nil, errors.New("等待状态超时")
}

func (c *gameConn) closeWithLogout() {
	if c == nil || c.raw == nil {
		return
	}
	_ = c.send(message{Type: typeLogout})
	_ = c.raw.Close()
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
		Labels      map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

func (t *tester) listPods() ([]pod, error) {
	raw, err := t.kubectlJSON("get", "pods", "-o", "json")
	if err != nil {
		return nil, err
	}
	var out podList
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (t *tester) firstRunningPodByApp(app string) (*pod, error) {
	pods, err := t.listPods()
	if err != nil {
		return nil, err
	}
	p := firstRunningPod(pods, app)
	if p == nil {
		return nil, fmt.Errorf("没有找到运行中的 Pod app=%s", app)
	}
	return p, nil
}

func firstRunningPod(pods []pod, app string) *pod {
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Metadata.Name < pods[j].Metadata.Name
	})
	for i := range pods {
		if pods[i].Status.Phase == "Running" && pods[i].Metadata.Labels["app"] == app {
			return &pods[i]
		}
	}
	return nil
}

func (t *tester) redisKeys(pattern string) ([]string, error) {
	out, err := t.kubectl("exec", "lab4-redis-0", "--", "redis-cli", "KEYS", pattern)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			keys = append(keys, line)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (t *tester) kubectlOK(args ...string) error {
	_, err := t.kubectl(args...)
	return err
}

func (t *tester) kubectlJSON(args ...string) ([]byte, error) {
	out, err := t.kubectl(args...)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

func (t *tester) kubectl(args ...string) (string, error) {
	full := []string{"-n", t.namespace}
	if len(args) > 0 && isClusterScoped(args[0], args[1:]...) {
		full = nil
	}
	full = append(full, args...)
	cmd := exec.Command(t.kubectlBin, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("kubectl %s 失败：%v\n%s", strings.Join(full, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func isClusterScoped(first string, rest ...string) bool {
	if first != "get" || len(rest) == 0 {
		return false
	}
	switch rest[0] {
	case "nodes", "node", "ns", "namespace", "namespaces":
		return true
	default:
		return false
	}
}

func selectCases(args []string) []testCase {
	if len(args) == 0 {
		return cases
	}
	lookup := make(map[string]testCase, len(cases)*2)
	for _, tc := range cases {
		lookup[tc.ID] = tc
		lookup[tc.Title] = tc
	}
	var selected []testCase
	for _, arg := range args {
		if tc, ok := lookup[arg]; ok {
			selected = append(selected, tc)
		}
	}
	if len(selected) == 0 {
		return cases
	}
	return selected
}

func retry(attempts int, delay time.Duration, fn func() error) error {
	var last error
	for i := 0; i < attempts; i++ {
		if err := fn(); err != nil {
			last = err
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return last
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

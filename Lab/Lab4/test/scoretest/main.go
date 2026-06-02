// Lab4 竞技评分脚本
//
// 目标：
//  1. 基础部署是否成功只做 preflight，不计入竞技分。
//  2. 分数主要来自 HPA 行为、故障恢复、一致性、资源纪律。
//  3. 尽量避免用学生电脑网络或硬件绝对性能打分，HPA 高压项会先做同集群自校准。
//
// 用法：
//
//	go run ./test/scoretest
//
// 常用环境变量：
//
//	LAB4_NAMESPACE=lab4
//	LAB4_GATEWAY_ADDR=120.79.8.174:30910
//	LAB4_SCORE_FAST=1              缩短测试时间，适合调试
//	LAB4_SKIP_CHAOS=1              跳过删除 Pod 的恢复测试
//	LAB4_SCORE_HIGH_CLIENTS=300    手动指定高压客户端数，跳过自校准
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"battleworld/protocol"
)

type config struct {
	namespace       string
	addr            string
	kubectl         string
	prefix          string
	fast            bool
	skipChaos       bool
	manualHighLoad  int
	idleWindow      time.Duration
	lowWindow       time.Duration
	highWindow      time.Duration
	calibrateWindow time.Duration
	progressEvery   time.Duration
}

type scoreItem struct {
	Name    string
	Points  float64
	Max     float64
	Waived  bool
	Reason  string
	Details []string
}

type scorer struct {
	cfg   config
	items []scoreItem
}

type loadResult struct {
	Started      int64
	AuthOK       int64
	OpsOK        int64
	RecvOK       int64
	DialErr      int64
	AuthErr      int64
	SendErr      int64
	ReadErr      int64
	ServerErr    int64
	MaxActive    int64
	Duration     time.Duration
	Observations []hpaObservation
}

type loadCounters struct {
	started   atomic.Int64
	authOK    atomic.Int64
	opsOK     atomic.Int64
	recvOK    atomic.Int64
	dialErr   atomic.Int64
	authErr   atomic.Int64
	sendErr   atomic.Int64
	readErr   atomic.Int64
	serverErr atomic.Int64
	active    atomic.Int64
	maxActive atomic.Int64
}

type hpaObservation struct {
	At              time.Time
	Replicas        map[string]int32
	DesiredReplicas map[string]int32
	MaxCPUPercent   map[string]int32
}

type hpaList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			MinReplicas *int32 `json:"minReplicas"`
			MaxReplicas int32  `json:"maxReplicas"`
			Metrics     []struct {
				Resource struct {
					Name   string `json:"name"`
					Target struct {
						AverageUtilization *int32 `json:"averageUtilization"`
					} `json:"target"`
				} `json:"resource"`
			} `json:"metrics"`
		} `json:"spec"`
		Status struct {
			CurrentReplicas int32 `json:"currentReplicas"`
			DesiredReplicas int32 `json:"desiredReplicas"`
			CurrentMetrics  []struct {
				Resource struct {
					Name    string `json:"name"`
					Current struct {
						AverageUtilization *int32 `json:"averageUtilization"`
					} `json:"current"`
				} `json:"resource"`
			} `json:"currentMetrics"`
		} `json:"status"`
	} `json:"items"`
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata struct {
		Name        string            `json:"name"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

type gameConn struct {
	raw *net.TCPConn
	enc *json.Encoder
	dec *json.Decoder
}

func main() {
	cfg := config{
		namespace:       env("LAB4_NAMESPACE", "lab4"),
		addr:            env("LAB4_GATEWAY_ADDR", "120.79.8.174:30910"),
		kubectl:         env("LAB4_KUBECTL", "kubectl"),
		prefix:          fmt.Sprintf("score-%d", time.Now().Unix()),
		fast:            env("LAB4_SCORE_FAST", "") == "1",
		skipChaos:       env("LAB4_SKIP_CHAOS", "") == "1",
		manualHighLoad:  envInt("LAB4_SCORE_HIGH_CLIENTS", 0),
		idleWindow:      30 * time.Second,
		lowWindow:       45 * time.Second,
		highWindow:      2 * time.Minute,
		calibrateWindow: 30 * time.Second,
		progressEvery:   5 * time.Second,
	}
	if cfg.fast {
		cfg.idleWindow = 10 * time.Second
		cfg.lowWindow = 20 * time.Second
		cfg.highWindow = 60 * time.Second
		cfg.calibrateWindow = 15 * time.Second
	}

	s := &scorer{cfg: cfg}
	fmt.Println("═══ BattleWorld Lab4 竞技评分 ═══")
	fmt.Printf("namespace=%s gateway=%s fast=%t skipChaos=%t\n\n", cfg.namespace, cfg.addr, cfg.fast, cfg.skipChaos)
	fmt.Printf("计时窗口：idle=%s low=%s calibrate=%s high=%s progress=%s\n\n",
		cfg.idleWindow, cfg.lowWindow, cfg.calibrateWindow, cfg.highWindow, cfg.progressEvery)

	s.logf("PRECHECK: 检查 Kubernetes 资源和游戏入口")
	if err := s.preflight(); err != nil {
		fmt.Printf("PRECHECK FAILED: %v\n", err)
		os.Exit(1)
	}
	s.logf("PRECHECK OK: 基础部署可用，开始竞技评分")
	fmt.Println()

	s.scoreIdleStability()
	s.scoreLowPressure()
	s.scoreHighPressure()
	s.scoreStateConsistency()
	if cfg.skipChaos {
		s.add(scoreItem{Name: "故障恢复", Max: 20, Waived: true, Reason: "LAB4_SKIP_CHAOS=1"})
	} else {
		s.scoreFaultRecovery()
	}
	s.scoreResourceDiscipline()
	s.print()
}

func (s *scorer) preflight() error {
	for _, name := range []string{
		"deployment/lab4-gateway",
		"deployment/lab4-coordinator",
		"deployment/lab4-map-green",
		"deployment/lab4-map-cave",
		"deployment/lab4-map-ruins",
		"statefulset/lab4-redis",
		"svc/lab4-gateway",
	} {
		if _, err := s.kubectl("get", name); err != nil {
			return err
		}
	}
	c, _, err := dialQuick(s.cfg.addr, s.cfg.prefix+"-preflight", "pw")
	if err != nil {
		return fmt.Errorf("游戏入口不可用：%w", err)
	}
	c.closeWithLogout()
	return nil
}

func (s *scorer) scoreIdleStability() {
	s.logf("阶段 1/6 空闲不误扩：观察 %s 内是否误扩容", s.cfg.idleWindow)
	before, err := s.observeHPA()
	if err != nil {
		s.add(scoreItem{Name: "空闲不误扩", Max: 10, Points: 0, Reason: err.Error()})
		s.logf("阶段 1/6 空闲不误扩：失败：%v", err)
		return
	}
	s.waitWithProgress("空闲观察", s.cfg.idleWindow)
	after, err := s.observeHPA()
	if err != nil {
		s.add(scoreItem{Name: "空闲不误扩", Max: 10, Points: 0, Reason: err.Error()})
		s.logf("阶段 1/6 空闲不误扩：失败：%v", err)
		return
	}
	excess := excessReplicas(after)
	points := clamp(10-float64(excess)*2, 0, 10)
	reason := "空闲阶段没有明显扩容"
	if excess > 0 {
		reason = fmt.Sprintf("空闲阶段存在 %d 个超出 minReplicas 的副本", excess)
	}
	s.add(scoreItem{
		Name:   "空闲不误扩",
		Max:    10,
		Points: points,
		Reason: reason,
		Details: []string{
			fmt.Sprintf("before=%v", before.Replicas),
			fmt.Sprintf("after=%v", after.Replicas),
		},
	})
	s.logf("阶段 1/6 空闲不误扩：完成，得分 %.1f/10", points)
}

func (s *scorer) scoreLowPressure() {
	s.logf("阶段 2/6 低压稳定性：clients=24 spawnRate=6 ops/s=1.0 duration=%s", s.cfg.lowWindow)
	before, _ := s.observeHPA()
	result := s.runLoad("low", 24, 6, 1.0, s.cfg.lowWindow)
	after := maxObservation(result.Observations)

	errRate := result.errorRate()
	points := 0.0
	details := []string{result.summary()}
	if errRate <= 0.01 {
		points += 6
	} else if errRate <= 0.03 {
		points += 3
	}
	growth := replicaGrowth(before, after)
	if growth <= 1 {
		points += 9
	} else if growth <= 2 {
		points += 5
	}
	s.add(scoreItem{
		Name:    "低压稳定性",
		Max:     15,
		Points:  clamp(points, 0, 15),
		Reason:  fmt.Sprintf("errorRate=%.2f%% replicaGrowth=%d", errRate*100, growth),
		Details: details,
	})
	s.logf("阶段 2/6 低压稳定性：完成，得分 %.1f/15，%s", clamp(points, 0, 15), result.summary())
}

func (s *scorer) scoreHighPressure() {
	s.logf("阶段 3/6 高压弹性扩容：先进行同集群自校准")
	clients, waived := s.calibrateHighLoad()
	if waived {
		s.add(scoreItem{
			Name:   "高压弹性扩容",
			Max:    25,
			Waived: true,
			Reason: "自校准阶段未形成明确 HPA 压力，避免把硬件/环境差异算作失分",
		})
		s.logf("阶段 3/6 高压弹性扩容：WAIVED，自校准未形成明确 HPA 压力")
		return
	}

	s.logf("阶段 3/6 高压弹性扩容：使用 clients=%d duration=%s", clients, s.cfg.highWindow)
	before, _ := s.observeHPA()
	result := s.runLoad("high", clients, max(10, clients/10), 4.0, s.cfg.highWindow)
	after := maxObservation(result.Observations)

	growth := replicaGrowth(before, after)
	desiredGrowth := desiredReplicaGrowth(before, after)
	componentGrowth := hpaComponentGrowth(before, after)
	errRate := result.errorRate()

	points := 0.0
	if growth >= 1 || desiredGrowth >= 1 {
		points += 10
	}
	if growth >= 3 || desiredGrowth >= 3 {
		points += 5
	} else if growth >= 2 || desiredGrowth >= 2 {
		points += 3
	}
	if componentGrowth >= 3 {
		points += 4
	} else if componentGrowth >= 2 {
		points += 2
	}
	if errRate <= 0.02 {
		points += 4
	} else if errRate <= 0.05 {
		points += 2
	}
	if result.OpsOK > int64(clients) {
		points += 2
	}
	s.add(scoreItem{
		Name:    "高压弹性扩容",
		Max:     25,
		Points:  clamp(points, 0, 25),
		Reason:  fmt.Sprintf("clients=%d replicaGrowth=%d desiredGrowth=%d components=%d errorRate=%.2f%%", clients, growth, desiredGrowth, componentGrowth, errRate*100),
		Details: []string{result.summary()},
	})
	s.logf("阶段 3/6 高压弹性扩容：完成，得分 %.1f/25，%s", clamp(points, 0, 25), result.summary())
}

func (s *scorer) calibrateHighLoad() (int, bool) {
	if s.cfg.manualHighLoad > 0 {
		s.logf("高压自校准：使用手动 clients=%d", s.cfg.manualHighLoad)
		return s.cfg.manualHighLoad, false
	}
	candidates := []int{40, 80, 160, 320}
	if s.cfg.fast {
		candidates = []int{30, 60, 120}
	}
	for _, clients := range candidates {
		s.logf("高压自校准：尝试 clients=%d spawnRate=%d duration=%s", clients, max(8, clients/10), s.cfg.calibrateWindow)
		result := s.runLoad("calibrate", clients, max(8, clients/10), 3.0, s.cfg.calibrateWindow)
		obs := maxObservation(result.Observations)
		s.logf("高压自校准：clients=%d 完成，maxCPU=%d desiredGrowth=%d errorRate=%.2f%%", clients, maxCPU(obs), maxDesiredGrowth(obs), result.errorRate()*100)
		if maxDesiredGrowth(obs) > 0 || maxCPU(obs) >= 45 || result.errorRate() > 0.03 {
			s.logf("高压自校准：选定 clients=%d", clients)
			return clients, false
		}
	}
	s.logf("高压自校准：所有候选都未形成明确压力")
	return candidates[len(candidates)-1], true
}

func (s *scorer) scoreStateConsistency() {
	s.logf("阶段 4/6 状态一致性：检查切图、移动、Redis、同账号接管")
	username := s.cfg.prefix + "-state"
	c, state, err := dialQuick(s.cfg.addr, username, "pw")
	if err != nil {
		s.add(scoreItem{Name: "状态一致性", Max: 20, Points: 0, Reason: err.Error()})
		s.logf("阶段 4/6 状态一致性：失败：%v", err)
		return
	}
	defer c.closeWithLogout()

	points := 0.0
	var details []string
	if len(state.Maps) >= 3 {
		points += 4
	}
	if st, err := c.sendAndWait(protocol.Message{Type: protocol.TypeSwitchMap, MapID: "cave"}, 5*time.Second); err == nil && st.Self.MapID == "cave" {
		points += 4
		details = append(details, "切图后 self/map 均恢复到 cave")
	}
	if _, err := c.sendAndWait(protocol.Message{Type: protocol.TypeMove, Dir: protocol.DirRight}, 5*time.Second); err == nil {
		points += 3
	}
	if keys, err := s.redisKeys("lab4*"); err == nil {
		need := []string{"lab4:user:" + username, "lab4:session:" + username, "lab4:checkpoint:cave"}
		matched := 0
		for _, key := range need {
			if contains(keys, key) {
				matched++
			}
		}
		points += float64(matched) * 2
		details = append(details, fmt.Sprintf("redisMatched=%d/%d", matched, len(need)))
	}
	if st, err := dialSecondLoginSnapshot(s.cfg.addr, username, "pw"); err == nil && onlineCount(st, username) == 1 {
		points += 3
		details = append(details, "同账号重连后在线列表不重复")
	}
	s.add(scoreItem{
		Name:    "状态一致性",
		Max:     20,
		Points:  clamp(points, 0, 20),
		Reason:  "检查切图、移动、Redis 热状态、同账号接管",
		Details: details,
	})
	s.logf("阶段 4/6 状态一致性：完成，得分 %.1f/20", clamp(points, 0, 20))
}

func (s *scorer) scoreFaultRecovery() {
	s.logf("阶段 5/6 故障恢复：删除业务 Pod 并检查恢复")
	total := 0.0
	var details []string
	for _, target := range []struct {
		name string
		app  string
		max  float64
	}{
		{name: "gateway", app: "lab4-gateway", max: 7},
		{name: "coordinator", app: "lab4-coordinator", max: 6},
		{name: "map-green", app: "lab4-map-green", max: 7},
	} {
		score, detail := s.deletePodAndCheckRecovery(target.name, target.app, target.max)
		total += score
		details = append(details, detail)
	}
	s.add(scoreItem{
		Name:    "故障恢复",
		Max:     20,
		Points:  clamp(total, 0, 20),
		Reason:  "删除业务 Pod 后检查会话恢复与状态保留",
		Details: details,
	})
	s.logf("阶段 5/6 故障恢复：完成，得分 %.1f/20", clamp(total, 0, 20))
}

func (s *scorer) deletePodAndCheckRecovery(label, app string, maxPoints float64) (float64, string) {
	s.logf("故障恢复/%s：准备登录并删除一个 %s Pod", label, app)
	username := fmt.Sprintf("%s-chaos-%s", s.cfg.prefix, strings.ReplaceAll(label, "-", ""))
	c, before, err := dialQuick(s.cfg.addr, username, "pw")
	if err != nil {
		return 0, fmt.Sprintf("%s: 登录失败 %v", label, err)
	}
	defer c.closeWithLogout()
	pod, err := s.firstRunningPod(app)
	if err != nil {
		return 0, fmt.Sprintf("%s: 找不到 Pod %v", label, err)
	}
	if _, err := s.kubectl("delete", "pod", pod.Metadata.Name, "--wait=false"); err != nil {
		return 0, fmt.Sprintf("%s: 删除 Pod 失败 %v", label, err)
	}
	s.logf("故障恢复/%s：已删除 Pod %s，等待 rollout", label, pod.Metadata.Name)
	if _, err := s.kubectl("rollout", "status", "deployment/"+app, "--timeout=240s"); err != nil {
		return maxPoints * 0.10, fmt.Sprintf("%s: rollout 未完成 %v", label, err)
	}
	points := maxPoints * 0.35
	s.logf("故障恢复/%s：rollout 完成，尝试重连并校验状态", label)
	next, after, err := reconnectUntil(s.cfg.addr, username, "pw", 60*time.Second)
	if err != nil {
		return points, fmt.Sprintf("%s: rollout 完成，但不能重连 %v", label, err)
	}
	next.closeWithLogout()
	points += maxPoints * 0.35
	if after.Self.Username == before.Self.Username && after.Self.HP > 0 && after.Self.MapID == before.Self.MapID {
		points += maxPoints * 0.20
	}
	if onlineCount(after, username) == 1 {
		points += maxPoints * 0.10
	}
	s.logf("故障恢复/%s：完成，得分 %.1f/%.1f", label, clamp(points, 0, maxPoints), maxPoints)
	return clamp(points, 0, maxPoints), fmt.Sprintf("%s: rollout=ok beforeMap=%s afterMap=%s online=%d", label, before.Self.MapID, after.Self.MapID, onlineCount(after, username))
}

func (s *scorer) scoreResourceDiscipline() {
	s.logf("阶段 6/6 资源纪律：检查 HPA、Redis、requests/limits、readiness/preStop")
	raw, err := s.kubectl("get", "hpa", "-o", "json")
	if err != nil {
		s.add(scoreItem{Name: "资源纪律", Max: 10, Points: 0, Reason: err.Error()})
		s.logf("阶段 6/6 资源纪律：失败：%v", err)
		return
	}
	var hpas hpaList
	if err := json.Unmarshal([]byte(raw), &hpas); err != nil {
		s.add(scoreItem{Name: "资源纪律", Max: 10, Points: 0, Reason: err.Error()})
		s.logf("阶段 6/6 资源纪律：失败：%v", err)
		return
	}
	points := 0.0
	details := []string{}
	for _, want := range businessApps() {
		if ok, detail := scoreHPAConfig(hpas, want); ok {
			points += 1
			details = append(details, detail)
		} else {
			details = append(details, detail)
		}
	}
	if _, err := s.kubectl("get", "hpa", "lab4-redis"); err != nil {
		points += 2
		details = append(details, "Redis 未配置 HPA")
	} else {
		details = append(details, "Redis 不应配置 HPA")
	}
	if err := s.checkDeploymentResources(); err == nil {
		points += 2
		details = append(details, "业务 Deployment requests/limits 完整")
	} else {
		details = append(details, err.Error())
	}
	if err := s.checkDeploymentLifecycle(); err == nil {
		points += 1
		details = append(details, "业务 Deployment readiness/preStop 完整")
	} else {
		details = append(details, err.Error())
	}
	s.add(scoreItem{
		Name:    "资源纪律",
		Max:     10,
		Points:  clamp(points, 0, 10),
		Reason:  "检查 HPA min/max、Redis 不做 HPA、requests/limits、readiness/preStop",
		Details: details,
	})
	s.logf("阶段 6/6 资源纪律：完成，得分 %.1f/10", clamp(points, 0, 10))
}

func (s *scorer) checkDeploymentResources() error {
	for _, name := range businessApps() {
		raw, err := s.kubectl("get", "deployment", name, "-o", "json")
		if err != nil {
			return err
		}
		var deploy struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Resources struct {
								Requests map[string]string `json:"requests"`
								Limits   map[string]string `json:"limits"`
							} `json:"resources"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		if err := json.Unmarshal([]byte(raw), &deploy); err != nil {
			return err
		}
		if len(deploy.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("%s 没有 container", name)
		}
		res := deploy.Spec.Template.Spec.Containers[0].Resources
		if res.Requests["cpu"] == "" || res.Requests["memory"] == "" || res.Limits["cpu"] == "" || res.Limits["memory"] == "" {
			return fmt.Errorf("%s requests/limits 不完整", name)
		}
	}
	return nil
}

func (s *scorer) checkDeploymentLifecycle() error {
	for _, name := range businessApps() {
		raw, err := s.kubectl("get", "deployment", name, "-o", "json")
		if err != nil {
			return err
		}
		var deploy struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							ReadinessProbe *struct{} `json:"readinessProbe"`
							Lifecycle      struct {
								PreStop *struct{} `json:"preStop"`
							} `json:"lifecycle"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		if err := json.Unmarshal([]byte(raw), &deploy); err != nil {
			return err
		}
		if len(deploy.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("%s 没有 container", name)
		}
		container := deploy.Spec.Template.Spec.Containers[0]
		if container.ReadinessProbe == nil || container.Lifecycle.PreStop == nil {
			return fmt.Errorf("%s readinessProbe/preStop 不完整", name)
		}
	}
	return nil
}

func (s *scorer) runLoad(name string, clients, spawnRate int, opsPerSecond float64, duration time.Duration) loadResult {
	s.logf("%s 压测：启动 clients=%d spawnRate=%d ops/s=%.1f duration=%s", loadLabel(name), clients, spawnRate, opsPerSecond, duration)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	startedAt := time.Now()
	counters := &loadCounters{}
	var wg sync.WaitGroup

	obsDone := make(chan []hpaObservation, 1)
	go func() {
		obsDone <- s.observeHPALoop(ctx, 5*time.Second)
	}()

	spawnInterval := time.Second / time.Duration(max(1, spawnRate))
	ticker := time.NewTicker(spawnInterval)
	defer ticker.Stop()
	spawned := 0
spawnLoop:
	for i := 0; i < clients; i++ {
		select {
		case <-ctx.Done():
			break spawnLoop
		case <-ticker.C:
			spawned++
			if spawned == clients || spawned%max(1, clients/4) == 0 {
				s.logf("%s 压测：已启动 %d/%d 个客户端", loadLabel(name), spawned, clients)
			}
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				s.loadClient(ctx, fmt.Sprintf("%s-%s-%04d", s.cfg.prefix, name, id), opsPerSecond, counters)
			}(i)
		}
	}
	progress := time.NewTicker(s.cfg.progressEvery)
	defer progress.Stop()
waitLoop:
	for {
		select {
		case <-ctx.Done():
			break waitLoop
		case <-progress.C:
			elapsed := time.Since(startedAt).Truncate(time.Second)
			remaining := duration - time.Since(startedAt)
			if remaining < 0 {
				remaining = 0
			}
			s.logf("%s 压测：进行中 elapsed=%s remaining=%s active=%d started=%d authOK=%d opsOK=%d recvOK=%d errors=%d",
				loadLabel(name),
				elapsed,
				remaining.Truncate(time.Second),
				counters.active.Load(),
				counters.started.Load(),
				counters.authOK.Load(),
				counters.opsOK.Load(),
				counters.recvOK.Load(),
				counters.dialErr.Load()+counters.authErr.Load()+counters.sendErr.Load()+counters.readErr.Load()+counters.serverErr.Load())
		}
	}
	wg.Wait()
	observations := <-obsDone

	result := loadResult{
		Started:      counters.started.Load(),
		AuthOK:       counters.authOK.Load(),
		OpsOK:        counters.opsOK.Load(),
		RecvOK:       counters.recvOK.Load(),
		DialErr:      counters.dialErr.Load(),
		AuthErr:      counters.authErr.Load(),
		SendErr:      counters.sendErr.Load(),
		ReadErr:      counters.readErr.Load(),
		ServerErr:    counters.serverErr.Load(),
		MaxActive:    counters.maxActive.Load(),
		Duration:     duration,
		Observations: observations,
	}
	s.logf("%s 压测：结束，%s", loadLabel(name), result.summary())
	return result
}

func (s *scorer) loadClient(ctx context.Context, username string, opsPerSecond float64, counters *loadCounters) {
	counters.started.Add(1)
	c, _, err := dialQuick(s.cfg.addr, username, "pw")
	if err != nil {
		if strings.Contains(err.Error(), "connect") || strings.Contains(err.Error(), "dial") {
			counters.dialErr.Add(1)
		} else {
			counters.authErr.Add(1)
		}
		return
	}
	defer c.closeWithLogout()
	counters.authOK.Add(1)
	active := counters.active.Add(1)
	updateMax(&counters.maxActive, active)
	defer counters.active.Add(-1)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			msg, err := c.recvWithin(2 * time.Second)
			if err != nil {
				if ctx.Err() == nil && !isTimeout(err) {
					counters.readErr.Add(1)
				}
				return
			}
			if msg.Type == protocol.TypeError {
				counters.serverErr.Add(1)
				continue
			}
			if msg.State != nil {
				counters.recvOK.Add(1)
			}
		}
	}()

	interval := time.Duration(float64(time.Second) / maxFloat(opsPerSecond, 0.1))
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	timer := time.NewTimer(jitter(interval, rng))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-readDone:
			return
		case <-timer.C:
			if err := c.send(randomMessage(rng)); err != nil {
				counters.sendErr.Add(1)
				return
			}
			counters.opsOK.Add(1)
			timer.Reset(jitter(interval, rng))
		}
	}
}

func (s *scorer) observeHPALoop(ctx context.Context, every time.Duration) []hpaObservation {
	var observations []hpaObservation
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if obs, err := s.observeHPA(); err == nil {
			observations = append(observations, obs)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return observations
		}
	}
}

func (s *scorer) observeHPA() (hpaObservation, error) {
	raw, err := s.kubectl("get", "hpa", "-o", "json")
	if err != nil {
		return hpaObservation{}, err
	}
	var list hpaList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return hpaObservation{}, err
	}
	obs := hpaObservation{
		At:              time.Now(),
		Replicas:        make(map[string]int32),
		DesiredReplicas: make(map[string]int32),
		MaxCPUPercent:   make(map[string]int32),
	}
	for _, item := range list.Items {
		obs.Replicas[item.Metadata.Name] = item.Status.CurrentReplicas
		obs.DesiredReplicas[item.Metadata.Name] = item.Status.DesiredReplicas
		for _, metric := range item.Status.CurrentMetrics {
			if metric.Resource.Name == "cpu" && metric.Resource.Current.AverageUtilization != nil {
				obs.MaxCPUPercent[item.Metadata.Name] = *metric.Resource.Current.AverageUtilization
			}
		}
	}
	return obs, nil
}

func maxObservation(observations []hpaObservation) hpaObservation {
	out := hpaObservation{
		Replicas:        make(map[string]int32),
		DesiredReplicas: make(map[string]int32),
		MaxCPUPercent:   make(map[string]int32),
	}
	for _, obs := range observations {
		for k, v := range obs.Replicas {
			if v > out.Replicas[k] {
				out.Replicas[k] = v
			}
		}
		for k, v := range obs.DesiredReplicas {
			if v > out.DesiredReplicas[k] {
				out.DesiredReplicas[k] = v
			}
		}
		for k, v := range obs.MaxCPUPercent {
			if v > out.MaxCPUPercent[k] {
				out.MaxCPUPercent[k] = v
			}
		}
	}
	return out
}

func (s *scorer) redisKeys(pattern string) ([]string, error) {
	out, err := s.kubectl("exec", "lab4-redis-0", "--", "redis-cli", "KEYS", pattern)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if key := strings.TrimSpace(line); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *scorer) firstRunningPod(app string) (*pod, error) {
	raw, err := s.kubectl("get", "pods", "-o", "json")
	if err != nil {
		return nil, err
	}
	var list podList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Metadata.Name < list.Items[j].Metadata.Name
	})
	for i := range list.Items {
		p := &list.Items[i]
		if p.Status.Phase == "Running" && p.Metadata.Labels["app"] == app {
			return p, nil
		}
	}
	return nil, fmt.Errorf("没有运行中的 Pod app=%s", app)
}

func (s *scorer) kubectl(args ...string) (string, error) {
	full := append([]string{"-n", s.cfg.namespace}, args...)
	cmd := exec.Command(s.cfg.kubectl, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kubectl %s 失败：%v\n%s", strings.Join(full, " "), err, string(out))
	}
	return string(out), nil
}

func dialQuick(addr, username, password string) (*gameConn, *protocol.WorldState, error) {
	return dialAuth(addr, protocol.TypeQuickEnter, username, password, password)
}

func dialLogin(addr, username, password string) (*gameConn, *protocol.WorldState, error) {
	return dialAuth(addr, protocol.TypeLogin, username, password, "")
}

func dialAuth(addr, typ, username, password, confirm string) (*gameConn, *protocol.WorldState, error) {
	raw, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	tcp := raw.(*net.TCPConn)
	c := &gameConn{raw: tcp, enc: json.NewEncoder(tcp), dec: json.NewDecoder(bufio.NewReader(tcp))}
	if err := c.send(protocol.Message{Type: typ, Username: username, Password: password, Confirm: confirm}); err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	msg, err := c.recvWithin(6 * time.Second)
	if err != nil {
		_ = c.raw.Close()
		return nil, nil, err
	}
	if msg.Type == protocol.TypeError || !msg.OK || msg.State == nil {
		_ = c.raw.Close()
		return nil, nil, fmt.Errorf("认证失败：%s", msg.Error)
	}
	return c, msg.State, nil
}

func reconnectUntil(addr, username, password string, timeout time.Duration) (*gameConn, *protocol.WorldState, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		c, state, err := dialLogin(addr, username, password)
		if err == nil {
			return c, state, nil
		}
		last = err
		time.Sleep(time.Second)
	}
	return nil, nil, last
}

func dialSecondLoginSnapshot(addr, username, password string) (*protocol.WorldState, error) {
	c, state, err := dialLogin(addr, username, password)
	if err != nil {
		return nil, err
	}
	c.closeWithLogout()
	return state, nil
}

func (c *gameConn) send(msg protocol.Message) error {
	return c.enc.Encode(msg)
}

func (c *gameConn) recvWithin(timeout time.Duration) (protocol.Message, error) {
	var msg protocol.Message
	_ = c.raw.SetReadDeadline(time.Now().Add(timeout))
	err := c.dec.Decode(&msg)
	_ = c.raw.SetReadDeadline(time.Time{})
	return msg, err
}

func (c *gameConn) sendAndWait(msg protocol.Message, timeout time.Duration) (*protocol.WorldState, error) {
	if err := c.send(msg); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		reply, err := c.recvWithin(time.Until(deadline))
		if err != nil {
			return nil, err
		}
		if reply.Type == protocol.TypeError {
			return nil, errors.New(reply.Error)
		}
		if reply.State != nil {
			return reply.State, nil
		}
	}
	return nil, errors.New("等待状态超时")
}

func (c *gameConn) closeWithLogout() {
	if c == nil || c.raw == nil {
		return
	}
	_ = c.send(protocol.Message{Type: protocol.TypeLogout})
	_ = c.raw.Close()
}

func randomMessage(rng *rand.Rand) protocol.Message {
	switch rng.Intn(6) {
	case 0, 1, 2:
		dirs := []string{protocol.DirUp, protocol.DirDown, protocol.DirLeft, protocol.DirRight}
		return protocol.Message{Type: protocol.TypeMove, Dir: dirs[rng.Intn(len(dirs))]}
	case 3:
		return protocol.Message{Type: protocol.TypeAttack}
	case 4:
		maps := []string{"green", "cave", "ruins"}
		return protocol.Message{Type: protocol.TypeSwitchMap, MapID: maps[rng.Intn(len(maps))]}
	default:
		return protocol.Message{Type: protocol.TypeHeal}
	}
}

func loadLabel(name string) string {
	switch name {
	case "low":
		return "低压"
	case "high":
		return "高压"
	case "calibrate":
		return "自校准"
	default:
		return name
	}
}

func (r loadResult) errorRate() float64 {
	errs := r.DialErr + r.AuthErr + r.SendErr + r.ReadErr + r.ServerErr
	total := r.Started + r.OpsOK + errs
	if total <= 0 {
		return 1
	}
	return float64(errs) / float64(total)
}

func (r loadResult) summary() string {
	return fmt.Sprintf("started=%d authOK=%d opsOK=%d recvOK=%d maxActive=%d errors=%d duration=%s",
		r.Started, r.AuthOK, r.OpsOK, r.RecvOK, r.MaxActive,
		r.DialErr+r.AuthErr+r.SendErr+r.ReadErr+r.ServerErr,
		r.Duration.Truncate(time.Second))
}

func (s *scorer) add(item scoreItem) {
	s.items = append(s.items, item)
}

func (s *scorer) logf(format string, args ...any) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func (s *scorer) waitWithProgress(label string, duration time.Duration) {
	start := time.Now()
	ticker := time.NewTicker(s.cfg.progressEvery)
	defer ticker.Stop()
	for {
		remaining := duration - time.Since(start)
		if remaining <= 0 {
			return
		}
		select {
		case <-time.After(remaining):
			return
		case <-ticker.C:
			elapsed := time.Since(start).Truncate(time.Second)
			left := duration - time.Since(start)
			if left < 0 {
				left = 0
			}
			s.logf("%s：进行中 elapsed=%s remaining=%s", label, elapsed, left.Truncate(time.Second))
		}
	}
}

func (s *scorer) print() {
	total, maxScore := 0.0, 0.0
	fmt.Println("═══ 评分明细 ═══")
	for _, item := range s.items {
		if item.Waived {
			fmt.Printf("WAIVED %-16s -- / %.0f  %s\n", item.Name, item.Max, item.Reason)
			continue
		}
		total += item.Points
		maxScore += item.Max
		fmt.Printf("%5.1f/%-5.1f %-16s %s\n", item.Points, item.Max, item.Name, item.Reason)
		for _, detail := range item.Details {
			fmt.Printf("              - %s\n", detail)
		}
	}
	fmt.Println()
	if maxScore == 0 {
		fmt.Println("总分：无可评分项目")
		return
	}
	fmt.Printf("总分：%.1f / %.1f（%.1f%%）\n", total, maxScore, total/maxScore*100)
}

func excessReplicas(obs hpaObservation) int {
	excess := 0
	for _, replicas := range obs.Replicas {
		if replicas > 1 {
			excess += int(replicas - 1)
		}
	}
	return excess
}

func replicaGrowth(before, after hpaObservation) int {
	growth := 0
	for name, afterReplicas := range after.Replicas {
		if beforeReplicas := before.Replicas[name]; afterReplicas > beforeReplicas {
			growth += int(afterReplicas - beforeReplicas)
		}
	}
	return growth
}

func desiredReplicaGrowth(before, after hpaObservation) int {
	growth := 0
	for name, afterReplicas := range after.DesiredReplicas {
		if beforeReplicas := before.DesiredReplicas[name]; afterReplicas > beforeReplicas {
			growth += int(afterReplicas - beforeReplicas)
		}
	}
	return growth
}

func hpaComponentGrowth(before, after hpaObservation) int {
	growth := 0
	for _, name := range businessApps() {
		if after.Replicas[name] > before.Replicas[name] || after.DesiredReplicas[name] > before.DesiredReplicas[name] {
			growth++
		}
	}
	return growth
}

func maxDesiredGrowth(obs hpaObservation) int {
	out := 0
	for name, desired := range obs.DesiredReplicas {
		if current := obs.Replicas[name]; desired > current {
			out += int(desired - current)
		}
	}
	return out
}

func maxCPU(obs hpaObservation) int32 {
	var out int32
	for _, value := range obs.MaxCPUPercent {
		if value > out {
			out = value
		}
	}
	return out
}

func onlineCount(state *protocol.WorldState, username string) int {
	if state == nil {
		return 0
	}
	count := 0
	for _, player := range state.OnlinePlayers {
		if player.Username == username {
			count++
		}
	}
	return count
}

func businessApps() []string {
	return []string{"lab4-gateway", "lab4-coordinator", "lab4-map-green", "lab4-map-cave", "lab4-map-ruins"}
}

func scoreHPAConfig(hpas hpaList, name string) (bool, string) {
	for _, hpa := range hpas.Items {
		if hpa.Metadata.Name != name {
			continue
		}
		minOK := hpa.Spec.MinReplicas != nil && *hpa.Spec.MinReplicas == 1
		maxOK := hpa.Spec.MaxReplicas == 10
		cpuOK := false
		for _, metric := range hpa.Spec.Metrics {
			if metric.Resource.Name == "cpu" && metric.Resource.Target.AverageUtilization != nil {
				target := *metric.Resource.Target.AverageUtilization
				cpuOK = target >= 40 && target <= 80
			}
		}
		if minOK && maxOK && cpuOK {
			return true, fmt.Sprintf("%s HPA min=1 max=10 cpuTarget 合理", name)
		}
		return false, fmt.Sprintf("%s HPA 配置不完整：minOK=%t maxOK=%t cpuOK=%t", name, minOK, maxOK, cpuOK)
	}
	return false, fmt.Sprintf("缺少 HPA %s", name)
}

func updateMax(target *atomic.Int64, value int64) {
	for {
		old := target.Load()
		if value <= old || target.CompareAndSwap(old, value) {
			return
		}
	}
}

func jitter(base time.Duration, rng *rand.Rand) time.Duration {
	return time.Duration(float64(base) * (0.8 + rng.Float64()*0.4))
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

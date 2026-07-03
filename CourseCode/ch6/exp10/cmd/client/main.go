package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	runtime := flag.String("runtime", "http://localhost:9000", "FaaS runtime URL")
	functionURL := flag.String("function-url", "", "Full function invoke URL (if provided, client will POST events directly to this URL)")
	functionName := flag.String("function", "daily_reward", "Function name when using local runtime")
	skipLoad := flag.Bool("skip-load", false, "Skip loading function to runtime (useful when invoking a pre-deployed/remote function)")
	flag.Parse()

	fmt.Println("=== Serverless 签到奖励实验 ===")
	fmt.Println()

	// Step 1: (Optional) Load function into runtime
	// If a direct function URL is provided we skip loading because the function
	// is assumed to be already deployed (e.g., on a remote cloud function service).
	if *functionURL == "" {
		if *skipLoad {
			fmt.Printf("[1] Skipping load step (skip-load=true). Assuming function %q already exists on runtime %s\n", *functionName, *runtime)
		} else {
			fmt.Printf("[1] 加载 %s 函数...\n", *functionName)
			loadPayload := map[string]interface{}{
				"name":      *functionName,
				"runtime":   "python",
				"handler":   "handler.py",
				"entry":     "handler",
				"memory_mb": 128,
				"timeout_s": 30,
			}
			body, _ := json.Marshal(loadPayload)
			resp, err := http.Post(*runtime+"/load", "application/json", bytes.NewReader(body))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			io.Copy(os.Stdout, resp.Body)
			resp.Body.Close()
			fmt.Println()
		}
	} else {
		fmt.Printf("[1] Using direct function URL: %s (skipping load on runtime)\n", *functionURL)
	}

	// Step 2: 冷启动调用
	fmt.Println("[2] 首次调用（冷启动）...")
	var invokeEndpoint string
	if *functionURL != "" {
		invokeEndpoint = *functionURL
	} else {
		invokeEndpoint = *runtime + "/invoke?function=" + *functionName
	}
	invokeAndPrint(invokeEndpoint, map[string]interface{}{
		"player_id": "player_001",
		"action":    "signin",
		"timestamp": time.Now().Format(time.RFC3339),
	})

	// Step 3: 热启动调用
	fmt.Println()
	fmt.Println("[3] 第二次调用（热启动）...")
	invokeAndPrint(invokeEndpoint, map[string]interface{}{
		"player_id": "player_002",
		"action":    "signin",
		"timestamp": time.Now().Format(time.RFC3339),
	})

	// Step 4: 查看统计
	// fmt.Println()
	// fmt.Println("[4] Runtime 统计...")
	// if *functionURL == "" {
	// 	statsResp, err := http.Get(*runtime + "/stats")
	// 	if err == nil {
	// 		io.Copy(os.Stdout, statsResp.Body)
	// 		statsResp.Body.Close()
	// 	}
	// } else {
	// 	fmt.Println("(Skipping runtime /stats because a direct function URL was provided)")
	// }
	// fmt.Println()

	// Step 5: 并发压测
	// fmt.Println()
	// fmt.Println("[5] 并发签到压测 (100 请求)...")
	// concurrentLoadTest(invokeEndpoint)
}

func invokeAndPrint(runtimeURL string, event map[string]interface{}) {
	body, _ := json.Marshal(event)
	start := time.Now()
	resp, err := http.Post(runtimeURL, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	duration := time.Since(start)
	defer resp.Body.Close()

	respBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", readErr)
		return
	}
	var result map[string]interface{}
	_ = json.Unmarshal(respBytes, &result)
	fmt.Printf("  Duration: %v\n", duration)
	if _, ok := result["body"]; ok {
		fmt.Printf("  Cold Start: %v\n", result["cold_start"])
		fmt.Printf("  Response: %s\n", prettyJSON(result["body"]))
		return
	}
	if len(result) > 0 {
		fmt.Printf("  Response: %s\n", prettyJSON(result))
		return
	}
	fmt.Printf("  Response (raw): %s\n", string(respBytes))
}

func concurrentLoadTest(runtimeURL string) {
	var wg sync.WaitGroup
	var totalOK, totalFail, totalCold int64
	start := time.Now()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := map[string]interface{}{
				"player_id": fmt.Sprintf("player_%03d", id),
				"action":    "signin",
				"timestamp": time.Now().Format(time.RFC3339),
			}
			body, _ := json.Marshal(event)
			resp, err := http.Post(runtimeURL, "application/json", bytes.NewReader(body))
			if err != nil {
				atomic.AddInt64(&totalFail, 1)
				return
			}
			defer resp.Body.Close()
			respBytes, _ := io.ReadAll(resp.Body)
			var result map[string]interface{}
			_ = json.Unmarshal(respBytes, &result)
			atomic.AddInt64(&totalOK, 1)
			if cold, ok := result["cold_start"].(bool); ok && cold {
				atomic.AddInt64(&totalCold, 1)
			}
		}(i)
	}
	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("  Total: 100\n")
	fmt.Printf("  OK: %d | Fail: %d\n", totalOK, totalFail)
	fmt.Printf("  Cold Starts: %d\n", totalCold)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  RPS: %.1f\n", 100/duration.Seconds())
}

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

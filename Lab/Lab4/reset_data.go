//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	baseDir := "student/data"
	dirs := []string{
		filepath.Join(baseDir, "cold"),
		filepath.Join(baseDir, "hot"),
		filepath.Join(baseDir, "hot", "checkpoints"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "创建目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	files := map[string]string{
		filepath.Join(baseDir, "cold", "users.json"):     "{}",
		filepath.Join(baseDir, "hot", "sessions.json"):   "{}",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已重置: %s\n", path)
	}

	// 清理检查点
	checkpointDir := filepath.Join(baseDir, "hot", "checkpoints")
	entries, err := os.ReadDir(checkpointDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(checkpointDir, entry.Name())
				_ = os.Remove(path)
				fmt.Printf("已删除: %s\n", path)
			}
		}
	}

	fmt.Println("数据重置完成。")
}

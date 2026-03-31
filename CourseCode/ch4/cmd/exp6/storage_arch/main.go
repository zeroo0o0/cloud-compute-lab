package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

type noopRedisLogger struct{}

func (noopRedisLogger) Printf(context.Context, string, ...interface{}) {}

type StorageDemo struct {
	db    *sql.DB
	redis *redis.Client
}

func defaultRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:6379"
}

func defaultPGDSN() string {
	if dsn := os.Getenv("PG_DSN"); dsn != "" {
		return dsn
	}
	return ""
}

func newStorageDemo(redisAddr, pgDSN string) (*StorageDemo, error) {
	redis.SetLogger(noopRedisLogger{})

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("连接 Redis %s 失败: %w", redisAddr, err)
	}

	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		return nil, fmt.Errorf("打开 PostgreSQL 失败: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}

	demo := &StorageDemo{db: db, redis: rdb}
	if err := demo.initSchema(); err != nil {
		_ = demo.Close()
		return nil, err
	}
	if err := demo.seedData(); err != nil {
		_ = demo.Close()
		return nil, err
	}

	return demo, nil
}

func (s *StorageDemo) initSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS players (
            user_id TEXT PRIMARY KEY,
            gold INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS game_configs (
            config_key TEXT PRIMARY KEY,
            config_value TEXT NOT NULL
        )`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化 PostgreSQL 表结构失败: %w", err)
		}
	}
	return nil
}

func (s *StorageDemo) seedData() error {
	if _, err := s.db.Exec(`
        INSERT INTO players (user_id, gold)
        VALUES ('player_1', 100)
        ON CONFLICT (user_id) DO UPDATE SET gold = EXCLUDED.gold`); err != nil {
		return fmt.Errorf("写入玩家初始数据失败: %w", err)
	}

	if _, err := s.db.Exec(`
        INSERT INTO game_configs (config_key, config_value)
        VALUES ('drop_rate', '1.5')
        ON CONFLICT (config_key) DO UPDATE SET config_value = EXCLUDED.config_value`); err != nil {
		return fmt.Errorf("写入配置初始数据失败: %w", err)
	}

	if err := s.redis.Set(ctx, "gold:player_1", "100", 0).Err(); err != nil {
		return fmt.Errorf("写入 Redis 初始金币失败: %w", err)
	}
	if err := s.redis.Del(ctx, "cfg:drop_rate").Err(); err != nil {
		return fmt.Errorf("清理 Redis 配置缓存失败: %w", err)
	}

	return nil
}

func (s *StorageDemo) Close() error {
	var firstErr error
	if err := s.redis.Close(); err != nil {
		firstErr = err
	}
	if err := s.db.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *StorageDemo) deductGold(userID string, deductAmount int) error {
	start := time.Now()
	fmt.Printf("[Write Through] 开始扣除 %s 金币 %d...\n", userID, deductAmount)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启 PostgreSQL 事务失败: %w", err)
	}

	var currentGold int
	err = tx.QueryRowContext(ctx, `
        UPDATE players
        SET gold = gold - $1
        WHERE user_id = $2
        RETURNING gold`, deductAmount, userID).Scan(&currentGold)
	if err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("玩家 %s 不存在", userID)
		}
		return fmt.Errorf("更新 PostgreSQL 金币失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 PostgreSQL 事务失败: %w", err)
	}

	if err := s.redis.Set(ctx, "gold:"+userID, fmt.Sprintf("%d", currentGold), 0).Err(); err != nil {
		return fmt.Errorf("同步 Redis 失败: %w", err)
	}

	fmt.Printf("[Write Through] 扣除成功，当前金币=%d，耗时=%v\n\n", currentGold, time.Since(start))
	return nil
}

func (s *StorageDemo) showGoldConsistency(userID string) {
	var dbGold int
	if err := s.db.QueryRowContext(ctx, `SELECT gold FROM players WHERE user_id = $1`, userID).Scan(&dbGold); err != nil {
		fmt.Printf("[一致性检查] PostgreSQL 查询失败: %v\n", err)
		return
	}

	cacheGold, err := s.redis.Get(ctx, "gold:"+userID).Result()
	if err != nil {
		fmt.Printf("[一致性检查] Redis 查询失败: %v\n", err)
		return
	}

	fmt.Printf("[一致性检查] PostgreSQL.gold=%d, Redis.gold=%s\n\n", dbGold, cacheGold)
}

func (s *StorageDemo) getGameConfig(key string) string {
	start := time.Now()
	cacheKey := "cfg:" + key

	val, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		fmt.Printf("[Cache Aside 读] 缓存命中 %s=%s，耗时=%v\n", key, val, time.Since(start))
		return val
	}

	fmt.Println("[Cache Aside 读] 缓存未命中，开始查询 PostgreSQL...")

	var dbVal string
	if err := s.db.QueryRowContext(ctx, `
        SELECT config_value FROM game_configs WHERE config_key = $1`, key).Scan(&dbVal); err != nil {
		fmt.Printf("[Cache Aside 读] PostgreSQL 查询失败: %v\n", err)
		return ""
	}

	if err := s.redis.Set(ctx, cacheKey, dbVal, 0).Err(); err != nil {
		fmt.Printf("[Cache Aside 读] Redis 回填失败: %v\n", err)
	}

	fmt.Printf("[Cache Aside 读] 已从 PostgreSQL 读取 %s=%s，耗时=%v\n\n", key, dbVal, time.Since(start))
	return dbVal
}

func (s *StorageDemo) updateGameConfig(key, newVal string) {
	start := time.Now()
	fmt.Printf("[Cache Aside 写] 开始更新 %s=%s ...\n", key, newVal)

	if _, err := s.db.ExecContext(ctx, `
        UPDATE game_configs
        SET config_value = $1
        WHERE config_key = $2`, newVal, key); err != nil {
		fmt.Printf("[Cache Aside 写] PostgreSQL 更新失败: %v\n", err)
		return
	}

	if err := s.redis.Del(ctx, "cfg:"+key).Err(); err != nil {
		fmt.Printf("[Cache Aside 写] 删除 Redis 缓存失败: %v\n", err)
		return
	}

	fmt.Printf("[Cache Aside 写] 更新成功，缓存已失效，耗时=%v\n\n", time.Since(start))
}

func printRunHints(redisAddr, pgDSN string) {
	fmt.Println("运行前置条件：")
	fmt.Printf("- Redis: %s（建议通过 Docker Desktop 启动）\n", redisAddr)
	if pgDSN == "" {
		fmt.Println("- PostgreSQL: 请先在 PowerShell 中设置 PG_DSN 环境变量")
	} else {
		fmt.Printf("- PostgreSQL: %s\n", pgDSN)
	}
	fmt.Println("- 程序会自动创建 players 和 game_configs 两张表，并写入演示初始数据。")
	fmt.Println()
}

func printInfraHelp() {
	fmt.Println("Redis 启动示例（先启动 Docker Desktop 再执行）：")
	fmt.Println(`docker run -d --name ch4-redis -p 6379:6379 redis:7`)
	fmt.Println()
	fmt.Println("PostgreSQL 启动示例（第一次创建）：")
	fmt.Println(`docker run -d --name ch4-postgres -e POSTGRES_USER=你的用户名 -e POSTGRES_PASSWORD=你的密码 -e POSTGRES_DB=postgres -p 5432:5432 postgres:16`)
	fmt.Println("如果容器已经创建过，可直接执行：docker start ch4-postgres")
	fmt.Println()
	fmt.Println("PostgreSQL 连接串示例：")
	fmt.Println(`$env:PG_DSN="postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable"`)
	fmt.Println("请把示例中的用户名、密码、主机、端口替换成你自己的 PostgreSQL 配置。")
}

func main() {
	fmt.Println("=== 实验六：分层存储架构（Redis + PostgreSQL） ===")
	fmt.Println("目标：使用真实 Redis 与 PostgreSQL 演示 Write Through 和 Cache Aside。")

	redisAddr := defaultRedisAddr()
	pgDSN := defaultPGDSN()
	printRunHints(redisAddr, pgDSN)
	if pgDSN == "" {
		fmt.Println("[错误] 未设置 PG_DSN。为了避免在代码里写死个人账号密码，实验六要求每位使用者自行配置 PostgreSQL 连接串。")
		printInfraHelp()
		return
	}

	demo, err := newStorageDemo(redisAddr, pgDSN)
	if err != nil {
		fmt.Printf("[错误] 初始化基础设施失败：%v\n", err)
		printInfraHelp()
		return
	}
	defer demo.Close()

	if err := demo.deductGold("player_1", 20); err != nil {
		fmt.Printf("[错误] Write Through 执行失败：%v\n", err)
		return
	}
	demo.showGoldConsistency("player_1")

	fmt.Println("--- 模拟配置读取与更新 ---")
	demo.getGameConfig("drop_rate")
	demo.getGameConfig("drop_rate")
	demo.updateGameConfig("drop_rate", "2.0")
	demo.getGameConfig("drop_rate")
	demo.getGameConfig("drop_rate")

	fmt.Println("[结论] 核心资产适合 Write Through，读多写少的配置数据适合 Cache Aside。")
}

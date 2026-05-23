package redisx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Addr     string
	Password string
	DB       int
	Timeout  time.Duration
}

func New(addr, password string, db int) *Client {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:6379"
	}
	return &Client{Addr: addr, Password: password, DB: db, Timeout: 3 * time.Second}
}

func FromEnv(defaultAddr string) *Client {
	db := 0
	if raw := strings.TrimSpace(os.Getenv("LAB4_REDIS_DB")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			db = parsed
		}
	}
	return New(firstNonEmpty(os.Getenv("LAB4_REDIS_ADDR"), defaultAddr), os.Getenv("LAB4_REDIS_PASSWORD"), db)
}

func (c *Client) Do(ctx context.Context, args ...string) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("redis command is empty")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	raw, err := dialer.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(timeout))

	rw := bufio.NewReadWriter(bufio.NewReader(raw), bufio.NewWriter(raw))
	if c.Password != "" {
		if err := writeCommand(rw, "AUTH", c.Password); err != nil {
			return nil, err
		}
		if _, err := readRESP(rw.Reader); err != nil {
			return nil, err
		}
	}
	if c.DB > 0 {
		if err := writeCommand(rw, "SELECT", strconv.Itoa(c.DB)); err != nil {
			return nil, err
		}
		if _, err := readRESP(rw.Reader); err != nil {
			return nil, err
		}
	}
	if err := writeCommand(rw, args...); err != nil {
		return nil, err
	}
	return readRESP(rw.Reader)
}

func (c *Client) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := c.Do(ctx, "GET", key)
	if err != nil {
		return "", false, err
	}
	if value == nil {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("redis GET %s returned %T", key, value)
	}
	return text, true, nil
}

func (c *Client) Set(ctx context.Context, key, value string) error {
	_, err := c.Do(ctx, "SET", key, value)
	return err
}

func (c *Client) SetEX(ctx context.Context, key, value string, ttl time.Duration) error {
	seconds := max(1, int(ttl.Seconds()))
	_, err := c.Do(ctx, "SET", key, value, "EX", strconv.Itoa(seconds))
	return err
}

func (c *Client) SetNXEX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	seconds := max(1, int(ttl.Seconds()))
	reply, err := c.Do(ctx, "SET", key, value, "NX", "EX", strconv.Itoa(seconds))
	if err != nil {
		return false, err
	}
	return reply != nil, nil
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	args := append([]string{"DEL"}, keys...)
	_, err := c.Do(ctx, args...)
	return err
}

func (c *Client) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	raw, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), target)
}

func (c *Client) SetJSON(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, string(data))
}

func writeCommand(rw *bufio.ReadWriter, args ...string) error {
	if _, err := fmt.Fprintf(rw, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(rw, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return rw.Flush()
}

func readRESP(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, errors.New(line)
	case ':':
		return strconv.ParseInt(line, 10, 64)
	case '$':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			value, err := readRESP(r)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown redis response prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

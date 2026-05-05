package migproto

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"time"
)

const (
	KiB = 1024
	MiB = 1024 * KiB
	// TotalStateBytes 表示模拟的完整玩家/进程状态大小，这里设为 50MB。
	TotalStateBytes int64 = 50 * MiB
	// DirtyPageBytes 表示预复制实验里单个脏页的模拟大小。
	DirtyPageBytes int64 = 1 * MiB
	// CriticalStateBytes 表示分波迁移里必须在停机窗口内传输的关键状态大小。
	// 这里设为 256KB：明显小于 Pre-Copy 的 1MB 最终脏页，但比 6KB 更容易观察到毫秒级停机时间。
	CriticalStateBytes int64 = 256 * KiB
)

// Event 是控制平面传输的迁移事件，用来记录当前阶段、数据量、停机时间等信息。
type Event struct {
	Mode             string  `json:"mode"`
	Stage            string  `json:"stage"`
	Message          string  `json:"message"`
	Bytes            int64   `json:"bytes,omitempty"`
	TotalTransferred int64   `json:"total_transferred,omitempty"`
	DowntimeMs       float64 `json:"downtime_ms,omitempty"`
	SentAt           string  `json:"sent_at"`
	Final            bool    `json:"final,omitempty"`
}

// HumanSize 把字节数转换成便于阅读的字符串，例如 1024 -> 1.00KB。
func HumanSize(bytes int64) string {
	if bytes >= MiB {
		return fmt.Sprintf("%.2fMB", float64(bytes)/float64(MiB))
	}
	if bytes >= KiB {
		return fmt.Sprintf("%.2fKB", float64(bytes)/float64(KiB))
	}
	return fmt.Sprintf("%dB", bytes)
}

// CalcTransferDuration 根据数据大小和给定吞吐量估算传输耗时。
func CalcTransferDuration(bytes int64, throughputBytesPerSec int64) time.Duration {
	if throughputBytesPerSec <= 0 {
		return 0
	}
	seconds := float64(bytes) / float64(throughputBytesPerSec)
	return time.Duration(seconds * float64(time.Second))
}

// NewEvent 创建一条迁移控制事件，自动填入发送时间。
func NewEvent(mode string, stage string, message string, bytes int64, totalTransferred int64, downtimeMs float64, final bool) Event {
	return Event{
		Mode:             mode,
		Stage:            stage,
		Message:          message,
		Bytes:            bytes,
		TotalTransferred: totalTransferred,
		DowntimeMs:       downtimeMs,
		SentAt:           time.Now().Format("15:04:05.000"),
		Final:            final,
	}
}

// SendEvent 把迁移事件编码成 JSON，并通过控制连接发送给对端。
func SendEvent(enc *json.Encoder, event Event) error {
	return enc.Encode(event)
}

// BuildPayload 构造指定大小的模拟状态数据。
func BuildPayload(size int64, seed byte) []byte {
	if size <= 0 {
		return nil
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = seed + byte(i%17)
	}
	return buf
}

// MeasureSerializeCopy 用完整内存拷贝模拟序列化/生成快照，并返回耗时和快照数据。
func MeasureSerializeCopy(src []byte) (time.Duration, []byte) {
	start := time.Now()
	out := make([]byte, len(src))
	copy(out, src)
	return time.Since(start), out
}

// MeasureApply 用计算 CRC32 模拟目标端反序列化/恢复状态，并返回耗时和校验值。
func MeasureApply(payload []byte) (time.Duration, uint32) {
	start := time.Now()
	sum := crc32.ChecksumIEEE(payload)
	return time.Since(start), sum
}

// TransferData 连接目标数据通道，把 payload 中的真实字节全部发送出去。
func TransferData(addr string, payload []byte) (time.Duration, int64, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()

	start := time.Now()
	remaining := payload
	var writtenTotal int64

	for len(remaining) > 0 {
		n, writeErr := conn.Write(remaining)
		if writeErr != nil {
			return time.Since(start), writtenTotal, writeErr
		}
		if n <= 0 {
			break
		}
		writtenTotal += int64(n)
		remaining = remaining[n:]
	}

	return time.Since(start), writtenTotal, nil
}

// TransferRealBytes 构造指定大小的模拟数据并发送，适合只关心传输体量的实验步骤。
func TransferRealBytes(addr string, totalBytes int64) (time.Duration, int64, error) {
	payload := BuildPayload(totalBytes, 0x3F)
	return TransferData(addr, payload)
}

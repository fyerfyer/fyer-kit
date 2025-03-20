package queueservice

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fyerfyer/fyer-kit/queue"
)

// QueueData 表示队列的可序列化数据结构
type QueueData struct {
	Name      string    `json:"name"`
	Type      QueueType `json:"type"`
	Capacity  int       `json:"capacity"`
	CreatedAt time.Time `json:"createdAt"`
	Items     []string  `json:"items,omitempty"`
}

// FormatQueueInfo 返回队列信息的格式化字符串表示
func FormatQueueInfo(info QueueInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Queue: %s\n", info.Name))
	sb.WriteString(fmt.Sprintf("Type: %s\n", info.Type))
	sb.WriteString(fmt.Sprintf("Size: %d", info.Stats.Size))
	if info.Stats.Capacity > 0 {
		sb.WriteString(fmt.Sprintf("/%d", info.Stats.Capacity))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Created: %s\n", formatTimeAgo(info.Stats.CreatedAt)))
	sb.WriteString(fmt.Sprintf("Operations: %d enqueued, %d dequeued\n",
		info.Stats.Enqueued, info.Stats.Dequeued))

	return sb.String()
}

// FormatQueueStats 返回队列统计信息的格式化字符串表示
func FormatQueueStats(stats queue.Stats) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Size: %d\n", stats.Size))
	if stats.Capacity > 0 {
		sb.WriteString(fmt.Sprintf("Capacity: %d (%.1f%% utilized)\n",
			stats.Capacity, stats.Utilization()*100))
	} else {
		sb.WriteString("Capacity: unbounded\n")
	}

	sb.WriteString(fmt.Sprintf("Created: %s\n", formatTimeAgo(stats.CreatedAt)))
	sb.WriteString(fmt.Sprintf("Operations: %d enqueued, %d dequeued\n",
		stats.Enqueued, stats.Dequeued))

	if stats.EnqueueBlocks > 0 || stats.DequeueBlocks > 0 {
		sb.WriteString(fmt.Sprintf("Blocks: %d enqueue, %d dequeue\n",
			stats.EnqueueBlocks, stats.DequeueBlocks))
	}

	if stats.EnqueueTimeouts > 0 || stats.DequeueTimeouts > 0 {
		sb.WriteString(fmt.Sprintf("Timeouts: %d enqueue, %d dequeue\n",
			stats.EnqueueTimeouts, stats.DequeueTimeouts))
	}

	if stats.Rejected > 0 {
		sb.WriteString(fmt.Sprintf("Rejected: %d\n", stats.Rejected))
	}

	return sb.String()
}

// SerializeQueueData 将队列数据序列化为JSON
func SerializeQueueData(data QueueData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// DeserializeQueueData 从JSON反序列化队列数据
func DeserializeQueueData(data []byte) (QueueData, error) {
	var queueData QueueData
	err := json.Unmarshal(data, &queueData)
	return queueData, err
}

// formatTimeAgo 将时间格式化为人类可读的"多久之前"字符串
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	seconds := int(duration.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%d seconds ago", seconds)
	}

	minutes := int(duration.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	hours := int(duration.Hours())
	if hours < 24 {
		return fmt.Sprintf("%d hours ago", hours)
	}

	days := int(duration.Hours() / 24)
	return fmt.Sprintf("%d days ago", days)
}

// ParseItems 解析以逗号分隔的项目字符串
func ParseItems(itemsStr string) []string {
	if itemsStr == "" {
		return nil
	}
	return strings.Split(itemsStr, ",")
}

// FormatItems 将项目切片格式化为以逗号分隔的字符串
func FormatItems(items []string) string {
	return strings.Join(items, ",")
}

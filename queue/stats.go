package queue

import "time"

// Stats 表示队列的统计信息
type Stats struct {
	// 创建时间
	CreatedAt time.Time

	// 队列容量
	Capacity int

	// 当前元素数量
	Size int

	// 入队操作次数
	Enqueued uint64

	// 出队操作次数
	Dequeued uint64

	// 入队阻塞计数
	EnqueueBlocks uint64

	// 出队阻塞计数
	DequeueBlocks uint64

	// 入队超时计数
	EnqueueTimeouts uint64

	// 出队超时计数
	DequeueTimeouts uint64

	// 拒绝的入队操作计数（队列已满或已关闭）
	Rejected uint64
}

// IsEmpty 返回队列是否为空
func (s *Stats) IsEmpty() bool {
	return s.Size == 0
}

// IsFull 返回队列是否已满
func (s *Stats) IsFull() bool {
	return s.Capacity > 0 && s.Size >= s.Capacity
}

// Utilization 返回队列利用率，范围从0到1
// 无界队列总是返回0
func (s *Stats) Utilization() float64 {
	if s.Capacity <= 0 {
		return 0
	}
	return float64(s.Size) / float64(s.Capacity)
}

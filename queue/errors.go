package queue

import "errors"

var (
	// ErrQueueClosed 表示队列已关闭
	ErrQueueClosed = errors.New("queue is closed")

	// ErrQueueFull 表示队列已满，无法添加更多元素
	ErrQueueFull = errors.New("queue is full")

	// ErrQueueEmpty 表示队列为空，无法获取元素
	ErrQueueEmpty = errors.New("queue is empty")

	// ErrOperationTimeout 表示操作超时
	ErrOperationTimeout = errors.New("operation timeout")

	// ErrOperationCancelled 表示操作被取消
	ErrOperationCancelled = errors.New("operation cancelled")

	// ErrInvalidCapacity 表示指定的队列容量无效
	ErrInvalidCapacity = errors.New("invalid queue capacity")

	// ErrNilItem 表示尝试入队空值
	ErrNilItem = errors.New("cannot enqueue nil item")
)

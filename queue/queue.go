package queue

import (
	"context"
)

// Queue 定义队列的基本操作接口
// 泛型参数T代表队列中存储的元素类型
type Queue[T any] interface {
	// Enqueue 将元素添加到队列尾部
	// 如果队列已满或已关闭，将返回错误
	// 如果提供了上下文且被取消，将返回错误
	Enqueue(ctx context.Context, item T) error

	// Dequeue 从队列头部移除并返回元素
	// 如果队列为空或已关闭，将返回错误
	// 如果提供了上下文且被取消，将返回错误
	Dequeue(ctx context.Context) (T, error)

	// TryEnqueue 尝试将元素添加到队列尾部，但不阻塞
	// 如果队列已满，将立即返回ErrQueueFull
	TryEnqueue(item T) error

	// TryDequeue 尝试从队列头部获取元素，但不阻塞
	// 如果队列为空，将立即返回ErrQueueEmpty
	TryDequeue() (T, error)

	// Peek 查看队列头部元素但不移除
	// 如果队列为空，将返回错误
	Peek() (T, error)

	// Size 返回队列当前元素数量
	Size() int

	// Capacity 返回队列容量，0表示无界队列
	Capacity() int

	// IsEmpty 检查队列是否为空
	IsEmpty() bool

	// IsFull 检查队列是否已满
	IsFull() bool

	// Close 关闭队列，不再接受新元素，已有元素可继续出队
	Close() error

	// IsClosed 检查队列是否已关闭
	IsClosed() bool

	// Clear 清空队列中的所有元素
	Clear()

	// Stats 返回队列的统计信息
	Stats() Stats
}

// NewQueue 创建一个新的阻塞队列
func NewQueue[T any](options ...Option) Queue[T] {
	// 实际队列实现在 blocking_queue.go 中
	return NewBlockingQueue[T](options...)
}

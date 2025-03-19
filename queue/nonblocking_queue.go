package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// NonBlockingQueue 实现了Queue接口的非阻塞版本
// 所有操作都不会阻塞调用线程，而是立即返回
type NonBlockingQueue[T any] struct {
	// 队列选项
	opts *Options

	// 内部数据存储
	data []T

	// 队列头部索引，使用原子操作访问
	head int32

	// 队列尾部索引，使用原子操作访问
	tail int32

	// 当前队列大小
	size int32

	// 队列是否已关闭
	closed atomic.Bool

	// 用于扩容和统计信息的互斥锁
	mu sync.RWMutex

	// 事件发射器
	events *EventEmitter

	// 统计信息
	stats Stats
}

// NewNonBlockingQueue 创建一个新的非阻塞队列实例
func NewNonBlockingQueue[T any](options ...Option) Queue[T] {
	opts := DefaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = 16 // 默认初始容量，无界队列会动态扩展
	}

	q := &NonBlockingQueue[T]{
		opts:  opts,
		data:  make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
		stats: Stats{CreatedAt: time.Now(), Capacity: opts.Capacity},
	}

	q.events = NewEventEmitter(opts.EventListeners)

	return q
}

// Enqueue 尝试将元素添加到队列尾部
// 在非阻塞队列中，此方法不会阻塞，而是立即返回结果
// 上下文参数仅用于检测取消
func (q *NonBlockingQueue[T]) Enqueue(ctx context.Context, item T) error {
	// 首先检查上下文是否已取消
	if ctx != nil && ctx.Err() != nil {
		q.emitErrorEvent(ErrOperationCancelled)
		return ErrOperationCancelled
	}

	// 检查队列是否已关闭
	if q.closed.Load() {
		q.incrementRejected()
		q.emitErrorEvent(ErrQueueClosed)
		return ErrQueueClosed
	}

	// 对于有界队列，检查是否已满
	if q.opts.Capacity > 0 && atomic.LoadInt32(&q.size) >= int32(q.opts.Capacity) {
		q.incrementRejected()
		q.emitErrorEvent(ErrQueueFull)
		return ErrQueueFull
	}

	// 无界队列且当前数组已满，需要扩容
	if q.opts.Capacity == 0 && atomic.LoadInt32(&q.size) >= int32(len(q.data)) {
		q.mu.Lock()
		// 再次检查，可能已经被其他线程扩容
		if atomic.LoadInt32(&q.size) >= int32(len(q.data)) {
			q.expand()
		}
		q.mu.Unlock()
	}

	// 尝试入队操作
	for {
		// 获取当前队列状态的快照
		tail := atomic.LoadInt32(&q.tail)
		size := atomic.LoadInt32(&q.size)
		capacity := int32(len(q.data))

		// 如果队列已满，则返回错误
		if q.opts.Capacity > 0 && size >= int32(q.opts.Capacity) {
			q.incrementRejected()
			q.emitErrorEvent(ErrQueueFull)
			return ErrQueueFull
		}

		// 计算新的索引和大小
		newTail := (tail + 1) % capacity
		newSize := size + 1

		// 尝试更新尾部索引
		if atomic.CompareAndSwapInt32(&q.tail, tail, newTail) {
			// 成功更新尾部索引，放入元素
			q.data[tail] = item

			// 更新大小
			atomic.AddInt32(&q.size, 1)

			// 更新统计信息
			q.incrementEnqueued()

			// 发送事件
			wasEmpty := size == 0
			if wasEmpty {
				q.events.Emit(Event{Type: EventEnqueue, Item: item, Size: int(newSize)})
			}

			isFull := q.opts.Capacity > 0 && newSize >= int32(q.opts.Capacity)
			if isFull {
				q.events.Emit(Event{Type: EventFull, Size: int(newSize)})
			}

			q.events.Emit(Event{Type: EventEnqueue, Item: item, Size: int(newSize)})

			return nil
		}

		// CAS失败，说明有竞争，重试
	}
}

// Dequeue 尝试从队列头部获取元素
// 在非阻塞队列中，此方法不会阻塞，而是立即返回结果
// 上下文参数仅用于检测取消
func (q *NonBlockingQueue[T]) Dequeue(ctx context.Context) (T, error) {
	var zero T

	// 首先检查上下文是否已取消
	if ctx != nil && ctx.Err() != nil {
		q.emitErrorEvent(ErrOperationCancelled)
		return zero, ErrOperationCancelled
	}

	// 检查队列是否为空
	if atomic.LoadInt32(&q.size) == 0 {
		if q.closed.Load() {
			return zero, ErrQueueClosed
		}
		return zero, ErrQueueEmpty
	}

	// 尝试出队操作
	for {
		// 获取当前队列状态的快照
		head := atomic.LoadInt32(&q.head)
		size := atomic.LoadInt32(&q.size)
		capacity := int32(len(q.data))

		// 如果队列为空，立即返回错误
		if size == 0 {
			if q.closed.Load() {
				return zero, ErrQueueClosed
			}
			return zero, ErrQueueEmpty
		}

		// 获取要出队的元素
		item := q.data[head]

		// 计算新的索引和大小
		newHead := (head + 1) % capacity
		newSize := size - 1

		// 尝试更新头部索引
		if atomic.CompareAndSwapInt32(&q.head, head, newHead) {
			// 成功更新头部索引
			// 清除原引用，帮助GC
			var zero T
			q.data[head] = zero

			// 更新大小
			atomic.AddInt32(&q.size, -1)

			// 更新统计信息
			q.incrementDequeued()

			// 发送事件
			wasFull := q.opts.Capacity > 0 && size >= int32(q.opts.Capacity)
			willBeEmpty := newSize == 0

			if wasFull {
				q.events.Emit(Event{Type: EventDequeue, Item: item, Size: int(newSize)})
			}

			if willBeEmpty {
				q.events.Emit(Event{Type: EventEmpty, Size: 0})
			}

			q.events.Emit(Event{Type: EventDequeue, Item: item, Size: int(newSize)})

			return item, nil
		}

		// CAS失败，说明有竞争，重试
	}
}

// TryEnqueue 尝试将元素添加到队列，不阻塞
func (q *NonBlockingQueue[T]) TryEnqueue(item T) error {
	// 在非阻塞队列中，TryEnqueue 和 Enqueue 实现相同
	return q.Enqueue(context.Background(), item)
}

// TryDequeue 尝试从队列获取元素，不阻塞
func (q *NonBlockingQueue[T]) TryDequeue() (T, error) {
	// 在非阻塞队列中，TryDequeue 和 Dequeue 实现相同
	return q.Dequeue(context.Background())
}

// Peek 查看队列头部元素但不移除
func (q *NonBlockingQueue[T]) Peek() (T, error) {
	var zero T

	q.mu.RLock()
	defer q.mu.RUnlock()

	// 检查队列是否为空
	if atomic.LoadInt32(&q.size) == 0 {
		if q.closed.Load() {
			return zero, ErrQueueClosed
		}
		return zero, ErrQueueEmpty
	}

	head := atomic.LoadInt32(&q.head)
	return q.data[head], nil
}

// Size 返回队列当前元素数量
func (q *NonBlockingQueue[T]) Size() int {
	return int(atomic.LoadInt32(&q.size))
}

// Capacity 返回队列容量
func (q *NonBlockingQueue[T]) Capacity() int {
	return q.opts.Capacity
}

// IsEmpty 检查队列是否为空
func (q *NonBlockingQueue[T]) IsEmpty() bool {
	return atomic.LoadInt32(&q.size) == 0
}

// IsFull 检查队列是否已满
func (q *NonBlockingQueue[T]) IsFull() bool {
	if q.opts.Capacity <= 0 {
		return false // 无界队列永远不会满
	}
	return atomic.LoadInt32(&q.size) >= int32(q.opts.Capacity)
}

// Close 关闭队列
func (q *NonBlockingQueue[T]) Close() error {
	// 尝试设置关闭标志
	if !q.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}

	// 发送关闭事件
	q.events.Emit(Event{Type: EventClose, Size: int(atomic.LoadInt32(&q.size))})

	return nil
}

// IsClosed 检查队列是否已关闭
func (q *NonBlockingQueue[T]) IsClosed() bool {
	return q.closed.Load()
}

// Clear 清空队列中的所有元素
func (q *NonBlockingQueue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 清空所有元素引用，帮助GC
	var zero T
	size := atomic.LoadInt32(&q.size)
	head := atomic.LoadInt32(&q.head)
	capacity := int32(len(q.data))

	for i := int32(0); i < size; i++ {
		idx := (head + i) % capacity
		q.data[idx] = zero
	}

	// 重置队列状态
	wasEmpty := size == 0
	wasFull := q.opts.Capacity > 0 && size >= int32(q.opts.Capacity)

	atomic.StoreInt32(&q.head, 0)
	atomic.StoreInt32(&q.tail, 0)
	atomic.StoreInt32(&q.size, 0)

	// 更新统计信息
	q.stats.Size = 0

	// 发送事件
	if wasFull {
		q.events.Emit(Event{Type: EventDequeue, Size: 0})
	}

	if !wasEmpty {
		q.events.Emit(Event{Type: EventEmpty, Size: 0})
	}
}

// Stats 返回队列的统计信息
func (q *NonBlockingQueue[T]) Stats() Stats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// 复制一份统计信息
	statsCopy := q.stats
	statsCopy.Size = int(atomic.LoadInt32(&q.size))
	return statsCopy
}

// 内部方法 - 扩展队列容量
func (q *NonBlockingQueue[T]) expand() {
	oldCapacity := len(q.data)
	newCapacity := oldCapacity * 2 // 翻倍扩容

	newData := make([]T, newCapacity)

	// 按顺序复制元素
	head := atomic.LoadInt32(&q.head)
	size := atomic.LoadInt32(&q.size)

	for i := int32(0); i < size; i++ {
		srcIndex := (head + i) % int32(oldCapacity)
		newData[i] = q.data[srcIndex]
	}

	// 更新队列数据
	atomic.StoreInt32(&q.head, 0)
	atomic.StoreInt32(&q.tail, size)
	q.data = newData
}

// 统计信息辅助方法
func (q *NonBlockingQueue[T]) incrementEnqueued() {
	q.mu.Lock()
	q.stats.Enqueued++
	q.stats.Size = int(atomic.LoadInt32(&q.size))
	q.mu.Unlock()
}

func (q *NonBlockingQueue[T]) incrementDequeued() {
	q.mu.Lock()
	q.stats.Dequeued++
	q.stats.Size = int(atomic.LoadInt32(&q.size))
	q.mu.Unlock()
}

func (q *NonBlockingQueue[T]) incrementRejected() {
	q.mu.Lock()
	q.stats.Rejected++
	q.mu.Unlock()
}

func (q *NonBlockingQueue[T]) emitErrorEvent(err error) {
	q.events.Emit(Event{Type: EventError, Err: err})
}

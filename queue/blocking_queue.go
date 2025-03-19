package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BlockingQueue 是Queue接口的具体实现
type BlockingQueue[T any] struct {
	// 队列选项
	opts *Options

	// 内部数据存储，使用切片实现
	data []T

	// 队列头部索引
	head int

	// 队列尾部索引
	tail int

	// 当前队列大小
	size int

	// 队列是否已关闭
	closed atomic.Bool

	// 保护队列操作的互斥锁
	mu sync.Mutex

	// 队列不为空条件变量，用于通知等待者队列有元素可出队
	notEmpty *sync.Cond

	// 队列不为满条件变量，用于通知等待者队列有空间可入队
	notFull *sync.Cond

	// 事件发射器
	events *EventEmitter

	// 统计信息
	stats Stats
}

// NewBlockingQueue 创建一个新的阻塞队列实例
func NewBlockingQueue[T any](options ...Option) Queue[T] {
	opts := DefaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = 16 // 默认初始容量，无界队列会动态扩展
	}

	q := &BlockingQueue[T]{
		opts:  opts,
		data:  make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
		stats: Stats{CreatedAt: time.Now(), Capacity: opts.Capacity},
	}

	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	q.events = NewEventEmitter(opts.EventListeners)

	return q
}

// Enqueue 将元素添加到队列尾部，如果队列已满则阻塞等待
func (q *BlockingQueue[T]) Enqueue(ctx context.Context, item T) error {
	// 首先检查上下文是否已取消
	if ctx != nil {
		select {
		case <-ctx.Done():
			q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
			return ErrOperationCancelled
		default:
		}
	}

	// 使用互斥锁保护队列操作
	q.mu.Lock()

	// 检查队列是否已关闭
	if q.closed.Load() {
		q.mu.Unlock()
		atomic.AddUint64(&q.stats.Rejected, 1)
		q.events.Emit(Event{Type: EventError, Err: ErrQueueClosed})
		return ErrQueueClosed
	}

	// 如果队列已满且是有界队列，需要等待
	if q.isFull() && q.opts.Capacity > 0 {
		// 记录等待次数
		atomic.AddUint64(&q.stats.EnqueueBlocks, 1)

		// 在等待之前检查是否有上下文和超时
		hasTimeout := q.opts.EnqueueTimeout > 0
		var timer *time.Timer
		var timerCh <-chan time.Time

		if hasTimeout {
			timer = time.NewTimer(q.opts.EnqueueTimeout)
			timerCh = timer.C
			defer timer.Stop()
		}

		// 周期性地检查条件和上下文状态
		for q.isFull() && !q.closed.Load() {
			// 创建一个通道用于通知取消状态
			waitCh := make(chan struct{})
			waitChClosed := false

			// 如果有上下文，启动一个 goroutine 监听上下文取消
			if ctx != nil {
				// 使用 sync.Once 确保通道只关闭一次
				var once sync.Once

				// 启动一个单独的 goroutine 来监听上下文取消
				go func() {
					select {
					case <-ctx.Done():
						// 获取锁并唤醒等待的 goroutine
						q.mu.Lock()
						// 安全地关闭通道
						once.Do(func() {
							close(waitCh)
							waitChClosed = true
						})
						q.mu.Unlock()
						q.notFull.Broadcast() // 唤醒所有等待者
					case <-waitCh:
						// 等待被其他方式唤醒
					}
				}()
			}

			// 释放锁并等待唤醒
			// 使用带超时的等待
			if hasTimeout {
				// 使用有限时间等待
				q.mu.Unlock()

				var timeoutOccurred bool
				var contextCancelled bool

				// 等待可能的事件：条件满足、超时、上下文取消
				select {
				case <-timerCh:
					timeoutOccurred = true
				case <-ctx.Done():
					contextCancelled = true
				case <-waitCh:
					// 被其他方式唤醒
				case <-time.After(50 * time.Millisecond): // 周期性唤醒检查条件
					// 时间到了，重新检查条件
				}

				// 重新获取锁继续
				q.mu.Lock()

				// 检查是什么导致我们被唤醒
				if timeoutOccurred {
					// 关闭等待通道，以免 goroutine 泄漏
					if !isClosed(waitCh) {
						close(waitCh)
					}
					q.mu.Unlock()
					atomic.AddUint64(&q.stats.EnqueueTimeouts, 1)
					q.events.Emit(Event{Type: EventError, Err: ErrOperationTimeout})
					return ErrOperationTimeout
				}

				if contextCancelled {
					// 关闭等待通道，以免 goroutine 泄漏
					if !isClosed(waitCh) {
						close(waitCh)
					}
					q.mu.Unlock()
					q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
					return ErrOperationCancelled
				}

				// 如果既没有超时也没有被取消，则继续检查条件
			} else {
				// 无超时等待
				// 使用短暂的等待时间，定期醒来检查上下文状态
				cond := sync.NewCond(&q.mu)
				go func() {
					time.Sleep(50 * time.Millisecond)
					cond.Signal() // 50ms后唤醒等待者
				}()
				cond.Wait() // 暂时等待

				// 检查上下文是否被取消
				if ctx != nil && ctx.Err() != nil {
					if !isClosed(waitCh) {
						close(waitCh)
					}
					q.mu.Unlock()
					q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
					return ErrOperationCancelled
				}

				// 否则继续检查队列条件
			}

			// 安全地关闭通道，确保不会重复关闭
			if !waitChClosed {
				close(waitCh)
			}
		}

		// 再次检查队列是否已关闭
		if q.closed.Load() {
			q.mu.Unlock()
			atomic.AddUint64(&q.stats.Rejected, 1)
			q.events.Emit(Event{Type: EventError, Err: ErrQueueClosed})
			return ErrQueueClosed
		}
	}

	// 如果队列已满且是无界队列，进行扩容
	if q.isFull() && q.opts.Capacity == 0 {
		q.expand()
	}

	// 将元素添加到队列中
	q.data[q.tail] = item
	q.tail = (q.tail + 1) % len(q.data)
	q.size++

	// 如果队列从空变为非空，通知等待的消费者
	if q.size == 1 {
		q.notEmpty.Signal()
	}

	// 更新统计信息并发送事件
	atomic.AddUint64(&q.stats.Enqueued, 1)
	q.stats.Size = q.size

	if q.isFull() {
		q.events.Emit(Event{Type: EventFull, Size: q.size})
	}

	q.events.Emit(Event{Type: EventEnqueue, Item: item, Size: q.size})

	q.mu.Unlock()
	return nil
}

// isClosed 检查通道是否已关闭
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Dequeue 从队列头部获取元素，如果队列为空则阻塞等待
func (q *BlockingQueue[T]) Dequeue(ctx context.Context) (T, error) {
	var zero T

	// 检查上下文是否已取消
	if ctx != nil {
		select {
		case <-ctx.Done():
			q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
			return zero, ErrOperationCancelled
		default:
		}
	}

	q.mu.Lock()

	// 检查队列是否已关闭且为空
	if q.isEmpty() && q.closed.Load() {
		q.mu.Unlock()
		return zero, ErrQueueClosed
	}

	// 如果队列为空，需要等待
	if q.isEmpty() {
		atomic.AddUint64(&q.stats.DequeueBlocks, 1)

		// 在等待之前检查是否有上下文和超时
		hasTimeout := q.opts.DequeueTimeout > 0
		var timer *time.Timer
		var timerCh <-chan time.Time

		if hasTimeout {
			timer = time.NewTimer(q.opts.DequeueTimeout)
			timerCh = timer.C
			defer timer.Stop()
		}

		// 周期性地检查条件和上下文状态
		for q.isEmpty() && !q.closed.Load() {
			// 注册一个能被唤醒的通道，用于周期性唤醒等待者以检查上下文状态
			waitCh := make(chan struct{})
			shouldCloseWaitCh := true // 添加一个标志，表示是否应该关闭waitCh

			// 如果有上下文，启动一个 goroutine 监听上下文取消
			var ctxDone <-chan struct{}
			if ctx != nil {
				ctxDone = ctx.Done()

				// 启动一个单独的 goroutine，但不在其中关闭waitCh
				go func(ch <-chan struct{}, waitCh chan struct{}) {
					select {
					case <-ch:
						// 上下文已取消，获取锁并唤醒等待的goroutine
						q.mu.Lock()
						q.notEmpty.Broadcast() // 唤醒所有等待者
						q.mu.Unlock()
					case <-waitCh:
						// 等待被其他方式唤醒，什么也不做
					}
				}(ctxDone, waitCh)
			}

			// 释放锁并等待唤醒
			// 使用带超时的等待
			if hasTimeout {
				// 使用有限时间等待
				q.mu.Unlock()

				var timeoutOccurred bool
				var contextCancelled bool

				// 等待可能的事件：条件满足、超时、上下文取消
				select {
				case <-timerCh:
					timeoutOccurred = true
				case <-ctx.Done():
					contextCancelled = true
				case <-waitCh:
					// 被其他方式唤醒
				case <-time.After(50 * time.Millisecond): // 周期性唤醒检查条件
					// 时间到了，重新检查条件
				}

				// 重新获取锁继续
				q.mu.Lock()

				// 检查是什么导致我们被唤醒
				if timeoutOccurred {

					close(waitCh)
					shouldCloseWaitCh = false

					q.mu.Unlock()
					atomic.AddUint64(&q.stats.DequeueTimeouts, 1)
					q.events.Emit(Event{Type: EventError, Err: ErrOperationTimeout})
					return zero, ErrOperationTimeout
				}

				if contextCancelled {
					if shouldCloseWaitCh {
						close(waitCh)
						shouldCloseWaitCh = false
					}
					q.mu.Unlock()
					q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
					return zero, ErrOperationCancelled
				}

				// 如果既没有超时也没有被取消，则继续检查条件
			} else {
				// 无超时等待
				// 使用短暂的等待时间，定期醒来检查上下文状态
				cond := sync.NewCond(&q.mu)
				go func() {
					time.Sleep(50 * time.Millisecond)
					cond.Signal() // 50ms后唤醒等待者
				}()
				cond.Wait() // 暂时等待

				// 检查上下文是否被取消
				if ctx != nil && ctx.Err() != nil {
					if shouldCloseWaitCh {
						close(waitCh)
						shouldCloseWaitCh = false
					}
					q.mu.Unlock()
					q.events.Emit(Event{Type: EventError, Err: ErrOperationCancelled})
					return zero, ErrOperationCancelled
				}

				// 否则继续检查队列条件
			}

			// 安全地关闭通道，确保不会重复关闭
			if shouldCloseWaitCh {
				close(waitCh)
			}
		}

		// 再次检查队列是否为空
		if q.isEmpty() {
			if q.closed.Load() {
				q.mu.Unlock()
				return zero, ErrQueueClosed
			}
			q.mu.Unlock()
			return zero, ErrQueueEmpty
		}
	}

	// 从队列中获取元素
	item := q.data[q.head]
	q.data[q.head] = zero // 清空引用，帮助GC
	q.head = (q.head + 1) % len(q.data)
	q.size--

	// 如果队列从满变为非满，通知等待的生产者
	wasFullBefore := q.opts.Capacity > 0 && q.size == q.opts.Capacity-1
	if wasFullBefore {
		q.notFull.Signal()
	}

	// 更新统计信息并发送事件
	atomic.AddUint64(&q.stats.Dequeued, 1)
	q.stats.Size = q.size

	isEmpty := q.isEmpty()
	if isEmpty {
		q.events.Emit(Event{Type: EventEmpty, Size: 0})
	}

	q.events.Emit(Event{Type: EventDequeue, Item: item, Size: q.size})

	q.mu.Unlock()
	return item, nil
}

// TryEnqueue 尝试将元素添加到队列，但不阻塞等待
func (q *BlockingQueue[T]) TryEnqueue(item T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 如果队列已关闭，拒绝入队
	if q.closed.Load() {
		atomic.AddUint64(&q.stats.Rejected, 1)
		q.events.Emit(Event{Type: EventError, Err: ErrQueueClosed})
		return ErrQueueClosed
	}

	// 如果队列已满且是有界队列，立即返回错误
	if q.isFull() && q.opts.Capacity > 0 {
		atomic.AddUint64(&q.stats.Rejected, 1)
		q.events.Emit(Event{Type: EventError, Err: ErrQueueFull})
		return ErrQueueFull
	}

	// 如果队列已满且是动态队列，进行扩容
	if q.isFull() && q.opts.Capacity == 0 {
		q.expand()
	}

	// 将元素添加到队列尾部
	q.data[q.tail] = item
	q.tail = (q.tail + 1) % len(q.data)
	q.size++

	// 队列从空变为非空时，通知等待出队的线程
	if q.size == 1 {
		q.notEmpty.Signal()
	}

	// 检查队列是否已满，发出事件通知
	if q.isFull() {
		q.events.Emit(Event{Type: EventFull, Size: q.size})
	}

	q.events.Emit(Event{Type: EventEnqueue, Item: item, Size: q.size})

	// 更新统计信息
	atomic.AddUint64(&q.stats.Enqueued, 1)
	q.stats.Size = q.size

	return nil
}

// TryDequeue 尝试从队列获取元素，但不阻塞等待
func (q *BlockingQueue[T]) TryDequeue() (T, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var zero T

	// 如果队列为空，立即返回错误
	if q.isEmpty() {
		if q.closed.Load() {
			return zero, ErrQueueClosed
		}
		return zero, ErrQueueEmpty
	}

	// 从队列头部获取元素
	item := q.data[q.head]
	// 清空引用，帮助GC
	q.data[q.head] = zero
	q.head = (q.head + 1) % len(q.data)
	q.size--

	// 队列从满变为非满时，通知等待入队的线程
	wasFullBefore := q.isFull()
	if wasFullBefore {
		q.notFull.Signal()
	}

	// 检查队列是否变为空，发出事件通知
	isEmpty := q.isEmpty()
	if isEmpty {
		q.events.Emit(Event{Type: EventEmpty, Size: q.size})
	}

	q.events.Emit(Event{Type: EventDequeue, Item: item, Size: q.size})

	// 更新统计信息
	atomic.AddUint64(&q.stats.Dequeued, 1)
	q.stats.Size = q.size

	return item, nil
}

// Peek 查看队列头部元素但不移除
func (q *BlockingQueue[T]) Peek() (T, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var zero T
	if q.isEmpty() {
		if q.closed.Load() {
			return zero, ErrQueueClosed
		}
		return zero, ErrQueueEmpty
	}

	return q.data[q.head], nil
}

// Size 返回队列当前元素数量
func (q *BlockingQueue[T]) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

// Capacity 返回队列容量
func (q *BlockingQueue[T]) Capacity() int {
	return q.opts.Capacity
}

// IsEmpty 检查队列是否为空
func (q *BlockingQueue[T]) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.isEmpty()
}

// IsFull 检查队列是否已满
func (q *BlockingQueue[T]) IsFull() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.isFull()
}

// Close 关闭队列
func (q *BlockingQueue[T]) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed.Load() {
		return nil // 已经关闭
	}

	q.closed.Store(true)
	q.events.Emit(Event{Type: EventClose, Size: q.size})

	// 唤醒所有等待者
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()

	return nil
}

// IsClosed 检查队列是否已关闭
func (q *BlockingQueue[T]) IsClosed() bool {
	return q.closed.Load()
}

// Clear 清空队列中的所有元素
func (q *BlockingQueue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 清空所有引用，帮助GC
	var zero T
	for i := 0; i < q.size; i++ {
		idx := (q.head + i) % len(q.data)
		q.data[idx] = zero
	}

	wasEmpty := q.isEmpty()
	wasFull := q.isFull()

	q.head = 0
	q.tail = 0
	q.size = 0

	if wasFull {
		q.notFull.Broadcast()
	}

	if !wasEmpty {
		q.events.Emit(Event{Type: EventEmpty, Size: 0})
	}

	q.stats.Size = 0
}

// Stats 返回队列的统计信息
func (q *BlockingQueue[T]) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 复制一份统计信息，避免外部修改
	statsCopy := q.stats
	return statsCopy
}

// 内部方法 - 检查队列是否为空
func (q *BlockingQueue[T]) isEmpty() bool {
	return q.size == 0
}

// 内部方法 - 检查队列是否已满
func (q *BlockingQueue[T]) isFull() bool {
	if q.opts.Capacity <= 0 {
		// 对于无界队列，当数组已满时，认为队列已满
		// 需要扩展，但从用户的角度来看并不是真的满了
		return q.size >= len(q.data)
	}
	return q.size >= q.opts.Capacity
}

// 内部方法 - 扩展队列容量
func (q *BlockingQueue[T]) expand() {
	oldCapacity := len(q.data)
	newCapacity := oldCapacity * 2 // 翻倍扩容

	// 打印扩容前后的状态，用于调试
	fmt.Printf("Expanding queue: old capacity=%d, new capacity=%d, head=%d, tail=%d, size=%d\n",
		oldCapacity, newCapacity, q.head, q.tail, q.size)

	newData := make([]T, newCapacity)

	// 按顺序复制元素
	for i := 0; i < q.size; i++ {
		srcIndex := (q.head + i) % oldCapacity
		newData[i] = q.data[srcIndex]

		// 打印每个被复制的元素
		if i < 5 || i >= q.size-5 {
			fmt.Printf("Copy element: from data[%d]=%v to newData[%d]\n",
				srcIndex, q.data[srcIndex], i)
		} else if i == 5 {
			fmt.Println("... (omitting middle elements) ...")
		}
	}

	q.head = 0
	q.tail = q.size
	q.data = newData

	fmt.Printf("After expansion: head=%d, tail=%d, size=%d\n", q.head, q.tail, q.size)
}

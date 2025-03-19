package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNonBlockingQueue_BasicOperations(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(5))

	// 测试入队和出队
	for i := 1; i <= 3; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("TryEnqueue(%d) failed: %v", i, err)
		}
	}

	// 检查队列大小
	if q.Size() != 3 {
		t.Fatalf("Expected size 3, got %d", q.Size())
	}

	// 检查Peek
	if val, err := q.Peek(); err != nil || val != 1 {
		t.Fatalf("Peek() expected 1, got %v (err: %v)", val, err)
	}

	// 测试出队
	for i := 1; i <= 3; i++ {
		val, err := q.TryDequeue()
		if err != nil {
			t.Fatalf("TryDequeue() failed: %v", err)
		}
		if val != i {
			t.Fatalf("Expected %d, got %d", i, val)
		}
	}

	// 检查队列是否为空
	if !q.IsEmpty() {
		t.Fatal("Queue should be empty")
	}

	// 测试空队列出队
	_, err := q.TryDequeue()
	if !errors.Is(err, ErrQueueEmpty) {
		t.Fatalf("Expected ErrQueueEmpty, got %v", err)
	}
}

func TestNonBlockingQueue_BoundedCapacity(t *testing.T) {
	capacity := 3
	q := NewNonBlockingQueue[string](WithCapacity(capacity))

	// 填充队列
	for i := 0; i < capacity; i++ {
		if err := q.TryEnqueue("item" + string(rune('A'+i))); err != nil {
			t.Fatalf("TryEnqueue failed: %v", err)
		}
	}

	// 检查队列是否已满
	if !q.IsFull() {
		t.Fatal("Queue should be full")
	}

	// 尝试向已满队列添加元素，应该立即返回错误
	err := q.TryEnqueue("overflow")
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Expected ErrQueueFull, got %v", err)
	}

	// 通过标准的Enqueue也应该立即返回错误，不会阻塞
	start := time.Now()
	err = q.Enqueue(context.Background(), "overflow")
	elapsed := time.Since(start)

	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Expected ErrQueueFull, got %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Fatalf("Enqueue on full nonblocking queue took too long: %v", elapsed)
	}

	// 出队一个元素
	val, err := q.TryDequeue()
	if err != nil {
		t.Fatalf("TryDequeue failed: %v", err)
	}
	if val != "itemA" {
		t.Fatalf("Expected itemA, got %v", val)
	}

	// 现在队列不应该满了
	if q.IsFull() {
		t.Fatal("Queue should not be full after dequeue")
	}

	// 应该可以再添加一个元素
	if err := q.TryEnqueue("newItem"); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}
}

func TestNonBlockingQueue_UnboundedCapacity(t *testing.T) {
	q := NewNonBlockingQueue[int]() // 默认无界队列

	// 添加大量元素，测试动态扩容
	count := 1000
	for i := 0; i < count; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("TryEnqueue(%d) failed: %v", i, err)
		}

		if i%100 == 0 {
			t.Logf("Enqueued %d elements, queue size: %d", i+1, q.Size())
		}
	}

	t.Logf("After enqueue: queue size: %d", q.Size())

	// 检查元素是否正确
	for i := 0; i < count; i++ {
		val, err := q.TryDequeue()
		if err != nil {
			t.Fatalf("TryDequeue() failed at %d: %v", i, err)
		}
		if val != i {
			t.Fatalf("Expected %d, got %d", i, val)
		}
		if i%100 == 0 {
			t.Logf("Dequeued %d elements, current element: %d", i+1, val)
		}
	}

	// 检查队列是否为空
	if !q.IsEmpty() {
		t.Fatal("Queue should be empty after dequeuing all items")
	}

	t.Logf("Test completed, queue size: %d", q.Size())
}

func TestNonBlockingQueue_Close(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(5))

	// 入队一些元素
	for i := 1; i <= 3; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("TryEnqueue(%d) failed: %v", i, err)
		}
	}

	// 关闭队列
	if err := q.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// 尝试入队，应该立即失败
	if err := q.TryEnqueue(4); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Expected ErrQueueClosed, got %v", err)
	}

	// 应该仍可以从队列中取出已有元素
	for i := 1; i <= 3; i++ {
		val, err := q.TryDequeue()
		if err != nil {
			t.Fatalf("TryDequeue() failed: %v", err)
		}
		if val != i {
			t.Fatalf("Expected %d, got %d", i, val)
		}
	}

	// 队列空且关闭，尝试出队应返回ErrQueueEmpty和ErrQueueClosed
	_, err := q.TryDequeue()
	if !errors.Is(err, ErrQueueEmpty) && !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Expected ErrQueueEmpty or ErrQueueClosed, got %v", err)
	}

	if !q.IsClosed() {
		t.Fatal("IsClosed() should return true")
	}
}

func TestNonBlockingQueue_Clear(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(5))

	// 入队一些元素
	for i := 1; i <= 3; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("TryEnqueue(%d) failed: %v", i, err)
		}
	}

	// 清空队列
	q.Clear()

	// 检查队列是否为空
	if !q.IsEmpty() {
		t.Fatal("Queue should be empty after Clear()")
	}

	// 检查大小是否为0
	if q.Size() != 0 {
		t.Fatalf("Expected size 0, got %d", q.Size())
	}

	// 清空后应该可以继续使用队列
	if err := q.TryEnqueue(42); err != nil {
		t.Fatalf("TryEnqueue after clear failed: %v", err)
	}
}

func TestNonBlockingQueue_NonBlockingBehavior(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(3))
	ctx := context.Background()

	// 填满队列
	for i := 1; i <= 3; i++ {
		if err := q.Enqueue(ctx, i); err != nil {
			t.Fatalf("Enqueue(%d) failed: %v", i, err)
		}
	}

	// 尝试入队到已满队列，应该立即返回错误而不阻塞
	start := time.Now()
	err := q.Enqueue(ctx, 4)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Expected ErrQueueFull, got %v", err)
	}

	// 确认是立即返回的，而不是阻塞等待
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Enqueue should return immediately, but took %v", elapsed)
	}

	// 清空队列
	q.Clear()

	// 尝试从空队列出队，应该立即返回错误而不阻塞
	start = time.Now()
	_, err = q.Dequeue(ctx)
	elapsed = time.Since(start)

	if !errors.Is(err, ErrQueueEmpty) {
		t.Fatalf("Expected ErrQueueEmpty, got %v", err)
	}

	// 确认是立即返回的，而不是阻塞等待
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Dequeue should return immediately, but took %v", elapsed)
	}
}

func TestNonBlockingQueue_ContextCancel(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(1))

	// 填满队列
	if err := q.TryEnqueue(1); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}

	// 创建一个已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 使用已取消的上下文入队，应立即返回错误
	err := q.Enqueue(ctx, 2)
	if !errors.Is(err, ErrOperationCancelled) {
		t.Fatalf("Expected ErrOperationCancelled, got %v", err)
	}

	// 清空队列
	q.Clear()

	// 使用已取消的上下文出队，应立即返回错误
	_, err = q.Dequeue(ctx)
	if !errors.Is(err, ErrOperationCancelled) {
		t.Fatalf("Expected ErrOperationCancelled, got %v", err)
	}
}

func TestNonBlockingQueue_ConcurrentAccess(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(1000))

	// 并发生产者/消费者计数
	producers := 5
	consumers := 5
	itemsPerProducer := 1000
	totalItems := producers * itemsPerProducer

	// 跟踪已处理的项目
	var consumedCount atomic.Int32
	consumedItems := sync.Map{}

	// 生产者等待组
	var producerWg sync.WaitGroup

	// 启动消费者
	var consumerWg sync.WaitGroup
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()

	// 用于信号通知消费者可以退出的通道
	done := make(chan struct{})
	defer close(done)

	// 启动消费者
	for i := 0; i < consumers; i++ {
		consumerWg.Add(1)
		go func(id int) {
			defer consumerWg.Done()

			for {
				select {
				case <-done:
					return
				case <-consumerCtx.Done():
					return
				default:
					// 继续消费
				}

				// 尝试出队，不会阻塞
				item, err := q.TryDequeue()
				if err == nil {
					// 成功出队
					if _, loaded := consumedItems.LoadOrStore(item, true); loaded {
						t.Errorf("Item %d consumed more than once", item)
					}

					newCount := consumedCount.Add(1)
					if newCount >= int32(totalItems) {
						return // 所有项目都已消费，可以退出
					}
				} else if errors.Is(err, ErrQueueEmpty) {
					// 队列暂时为空，短暂等待，给生产者机会
					time.Sleep(1 * time.Millisecond)
				} else if errors.Is(err, ErrQueueClosed) {
					// 队列已关闭，退出
					return
				}
			}
		}(i)
	}

	// 启动生产者
	for i := 0; i < producers; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()

			baseValue := producerID * itemsPerProducer
			for j := 0; j < itemsPerProducer; j++ {
				item := baseValue + j

				// 不断尝试入队，直到成功
				for {
					err := q.TryEnqueue(item)
					if err == nil {
						break // 成功入队
					} else if errors.Is(err, ErrQueueFull) {
						// 队列已满，短暂等待，给消费者机会
						time.Sleep(1 * time.Millisecond)
					} else {
						t.Errorf("Producer %d: unexpected error: %v", producerID, err)
						return
					}
				}
			}
		}(i)
	}

	// 等待所有生产者完成
	producerWg.Wait()

	// 等待所有项目被消费完
	waitWithTimeout := func(wg *sync.WaitGroup, timeout time.Duration) bool {
		c := make(chan struct{})
		go func() {
			defer close(c)
			wg.Wait()
		}()

		select {
		case <-c:
			return true // 正常完成
		case <-time.After(timeout):
			return false // 超时
		}
	}

	// 如果5秒内没有消费完所有项目，发送终止信号并报告
	if !waitWithTimeout(&consumerWg, 5*time.Second) {
		consumerCancel()
		t.Logf("Warning: Not all items consumed in time. Consumed: %d/%d",
			consumedCount.Load(), totalItems)
	}

	// 验证结果
	consumed := consumedCount.Load()
	if consumed != int32(totalItems) {
		t.Errorf("Expected %d consumed items, got %d", totalItems, consumed)
	}

	// 统计已消费的唯一项目数
	var uniqueCount int
	consumedItems.Range(func(_, _ interface{}) bool {
		uniqueCount++
		return true
	})

	if uniqueCount != totalItems {
		t.Errorf("Expected %d unique consumed items, got %d", totalItems, uniqueCount)
	}
}

func TestNonBlockingQueue_Stats(t *testing.T) {
	q := NewNonBlockingQueue[int](WithCapacity(3))
	ctx := context.Background()

	// 执行一些操作
	q.Enqueue(ctx, 1)
	q.Enqueue(ctx, 2)
	q.Enqueue(ctx, 3)

	// 这应该会立即返回失败
	_ = q.Enqueue(ctx, 4) // 应该得到 ErrQueueFull

	val, _ := q.Dequeue(ctx)
	if val != 1 {
		t.Fatalf("Expected 1, got %d", val)
	}

	// 获取统计信息
	stats := q.Stats()

	// 检查统计数据
	if stats.Size != 2 {
		t.Fatalf("Expected size 2, got %d", stats.Size)
	}

	if stats.Capacity != 3 {
		t.Fatalf("Expected capacity 3, got %d", stats.Capacity)
	}

	if stats.Enqueued < 3 {
		t.Fatalf("Expected at least 3 enqueued operations, got %d", stats.Enqueued)
	}

	if stats.Dequeued != 1 {
		t.Fatalf("Expected 1 dequeued operation, got %d", stats.Dequeued)
	}

	if stats.Rejected < 1 {
		t.Fatalf("Expected at least 1 rejected operation, got %d", stats.Rejected)
	}

	if stats.Utilization() != float64(2)/float64(3) {
		t.Fatalf("Expected utilization %.2f, got %.2f", float64(2)/float64(3), stats.Utilization())
	}
}

func TestNonBlockingQueue_Events(t *testing.T) {
	// 事件计数器
	var enqueueCalls, dequeueCalls, emptyCalls, fullCalls, closeCalls, errorCalls int

	// 事件监听器
	listener := func(evt Event) {
		switch evt.Type {
		case EventEnqueue:
			enqueueCalls++
		case EventDequeue:
			dequeueCalls++
		case EventEmpty:
			emptyCalls++
		case EventFull:
			fullCalls++
		case EventClose:
			closeCalls++
		case EventError:
			errorCalls++
		}
	}

	// 创建带监听器的队列
	q := NewNonBlockingQueue[int](WithCapacity(2), WithEventListener(listener))
	ctx := context.Background()

	// 入队两个元素，填满队列
	q.Enqueue(ctx, 1)
	q.Enqueue(ctx, 2)

	// 尝试入队第三个元素（队列已满）
	err := q.TryEnqueue(3)
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Expected ErrQueueFull, got %v", err)
	}

	// 出队所有元素，清空队列
	q.Dequeue(ctx)
	q.Dequeue(ctx)

	// 关闭队列
	q.Close()

	// 验证事件计数
	if enqueueCalls < 2 {
		t.Fatalf("Expected at least 2 enqueue events, got %d", enqueueCalls)
	}

	if dequeueCalls < 2 {
		t.Fatalf("Expected at least 2 dequeue events, got %d", dequeueCalls)
	}

	if fullCalls < 1 {
		t.Fatalf("Expected at least 1 full event, got %d", fullCalls)
	}

	if emptyCalls < 1 {
		t.Fatalf("Expected at least 1 empty event, got %d", emptyCalls)
	}

	if closeCalls != 1 {
		t.Fatalf("Expected 1 close event, got %d", closeCalls)
	}

	if errorCalls < 1 {
		t.Fatalf("Expected at least 1 error event, got %d", errorCalls)
	}
}

func TestNonBlockingQueue_StressTest(t *testing.T) {
	// 跳过此测试，除非使用 -stress 标志
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// 创建一个较小容量的队列以增加竞争
	q := NewNonBlockingQueue[int](WithCapacity(100))

	// 并发工作协程数
	goroutines := 20

	// 每个协程的操作次数
	opsPerGoroutine := 10000

	// 创建等待组
	var wg sync.WaitGroup

	// 统计成功操作
	var enqueueSuccess atomic.Int64
	var dequeueSuccess atomic.Int64

	// 启动工作协程
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 使用本地随机数生成器
			seed := time.Now().UnixNano() + int64(id)
			r := NewTestRand(seed)

			for j := 0; j < opsPerGoroutine; j++ {
				// 随机选择操作：50%概率入队，50%概率出队
				op := r.Intn(2)

				if op == 0 {
					// 入队操作
					if err := q.TryEnqueue(r.Intn(1000000)); err == nil {
						enqueueSuccess.Add(1)
					}
				} else {
					// 出队操作
					if _, err := q.TryDequeue(); err == nil {
						dequeueSuccess.Add(1)
					}
				}
			}
		}(i)
	}

	// 等待所有协程完成
	wg.Wait()

	t.Logf("Stress test completed")
	t.Logf("Successful enqueues: %d", enqueueSuccess.Load())
	t.Logf("Successful dequeues: %d", dequeueSuccess.Load())
	t.Logf("Final queue size: %d", q.Size())

	// 最终队列大小应该等于入队成功次数减去出队成功次数
	expectedSize := enqueueSuccess.Load() - dequeueSuccess.Load()
	if expectedSize >= 0 && q.Size() != int(expectedSize) {
		t.Errorf("Queue size mismatch: expected %d, got %d", expectedSize, q.Size())
	}
}

// 简单的随机数生成器，避免在并发环境中使用全局随机数生成器
type TestRand struct {
	state int64
}

func NewTestRand(seed int64) *TestRand {
	return &TestRand{state: seed}
}

func (r *TestRand) Intn(n int) int {
	r.state = (r.state*1103515245 + 12345) & 0x7fffffff
	return int(r.state % int64(n))
}

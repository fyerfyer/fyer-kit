package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockingQueue_BasicOperations(t *testing.T) {
	q := NewQueue[int](WithCapacity(5))

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

func TestBlockingQueue_BoundedCapacity(t *testing.T) {
	capacity := 3
	q := NewQueue[string](WithCapacity(capacity))

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

	// 尝试向已满队列添加元素
	err := q.TryEnqueue("overflow")
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Expected ErrQueueFull, got %v", err)
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
		t.Fatal("Queue should not be full")
	}

	// 应该可以再添加一个元素
	if err := q.TryEnqueue("newItem"); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}
}

func TestBlockingQueue_UnboundedCapacity(t *testing.T) {
	q := NewQueue[int]() // 默认无界队列
	bq := q.(*BlockingQueue[int])
	t.Logf("Created unbounded queue with initial capacity: %d", len(bq.data))

	// 添加大量元素，测试动态扩容
	count := 1000
	for i := 0; i < count; i++ {
		if err := q.TryEnqueue(i); err != nil {
			t.Fatalf("TryEnqueue(%d) failed: %v", i, err)
		}

		prevCapacity := len(bq.data)

		// 每次扩容时打印详细信息
		if i > 0 && len(bq.data) > prevCapacity {
			t.Logf("Queue expanded from %d to %d at i=%d", prevCapacity, len(bq.data), i)
			t.Logf("Queue state after expansion: head=%d, tail=%d, size=%d",
				bq.head, bq.tail, bq.size)

			// 打印队列的前几个和后几个元素来验证
			t.Log("First few elements in queue:")
			for j := 0; j < min(5, bq.size); j++ {
				idx := (bq.head + j) % len(bq.data)
				t.Logf("  data[%d] (logical %d) = %v", idx, j, bq.data[idx])
			}

			t.Log("Last few elements in queue:")
			for j := max(0, bq.size-5); j < bq.size; j++ {
				idx := (bq.head + j) % len(bq.data)
				t.Logf("  data[%d] (logical %d) = %v", idx, j, bq.data[idx])
			}
		}

		if i%100 == 0 {
			t.Logf("Enqueued %d elements, queue size: %d, capacity: %d, head: %d, tail: %d",
				i+1, q.Size(), len(bq.data), bq.head, bq.tail)
		}
	}

	t.Logf("After enqueue: queue size: %d, head: %d, tail: %d, data length: %d",
		q.Size(), bq.head, bq.tail, len(bq.data))

	// 验证队列的顺序
	t.Log("Verifying first 10 elements:")
	for i := 0; i < min(10, bq.size); i++ {
		idx := (bq.head + i) % len(bq.data)
		t.Logf("  Expected: %d, Actual: %d", i, bq.data[idx])
		if bq.data[idx] != i {
			t.Errorf("  Element at position %d is incorrect: expected %d, got %d",
				i, i, bq.data[idx])
		}
	}

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

	t.Logf("Test completed, queue size: %d", q.Size())
}

func TestBlockingQueue_Close(t *testing.T) {
	q := NewQueue[int](WithCapacity(5))

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

	// 尝试入队，应该失败
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

	// 队列空且关闭，尝试出队应返回ErrQueueClosed
	_, err := q.TryDequeue()
	if !errors.Is(err, ErrQueueEmpty) && !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Expected ErrQueueEmpty or ErrQueueClosed, got %v", err)
	}

	if !q.IsClosed() {
		t.Fatal("IsClosed() should return true")
	}
}

func TestBlockingQueue_Clear(t *testing.T) {
	q := NewQueue[int](WithCapacity(5))

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

func TestBlockingQueue_BlockingEnqueueDequeue(t *testing.T) {
	q := NewQueue[int](WithCapacity(3))
	ctx := context.Background()

	// 填满队列
	for i := 1; i <= 3; i++ {
		if err := q.Enqueue(ctx, i); err != nil {
			t.Fatalf("Enqueue(%d) failed: %v", i, err)
		}
	}

	// 启动一个goroutine阻塞入队
	var enqueueErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		enqueueErr = q.Enqueue(ctx, 4) // 应该阻塞
	}()

	// 稍等一会，确保goroutine已经阻塞
	time.Sleep(100 * time.Millisecond)

	// 出队一个元素，应该解除阻塞
	val, err := q.Dequeue(ctx)
	if err != nil || val != 1 {
		t.Fatalf("Dequeue() expected 1, got %v (err: %v)", val, err)
	}

	// 等待入队goroutine完成
	wg.Wait()

	// 检查入队是否成功
	if enqueueErr != nil {
		t.Fatalf("Blocking Enqueue failed: %v", enqueueErr)
	}

	// 队列应该再次已满
	if !q.IsFull() {
		t.Fatal("Queue should be full again")
	}

	// 验证队列内容：应该是2, 3, 4
	expected := []int{2, 3, 4}
	for i, exp := range expected {
		val, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue()[%d] failed: %v", i, err)
		}
		if val != exp {
			t.Fatalf("Expected %d, got %d", exp, val)
		}
	}
}

func TestBlockingQueue_Timeout(t *testing.T) {
	// 创建有超时的队列
	timeout := 200 * time.Millisecond
	q := NewQueue[int](
		WithCapacity(1),
		WithEnqueueTimeout(timeout),
		WithDequeueTimeout(timeout),
	)

	// 填满队列
	if err := q.TryEnqueue(1); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}

	// 创建一个带超时的上下文
	ctx := context.Background()

	// 尝试入队，应该超时
	startTime := time.Now()
	err := q.Enqueue(ctx, 2)
	elapsed := time.Since(startTime)

	if !errors.Is(err, ErrOperationTimeout) {
		t.Fatalf("Expected ErrOperationTimeout, got %v", err)
	}

	// 检查是否大致等待了超时时间
	if elapsed < timeout {
		t.Fatalf("Timeout too short: %v", elapsed)
	}

	// 清空队列
	q.Clear()

	// 尝试出队空队列，应该超时
	startTime = time.Now()
	_, err = q.Dequeue(ctx)
	elapsed = time.Since(startTime)

	if !errors.Is(err, ErrOperationTimeout) {
		t.Fatalf("Expected ErrOperationTimeout, got %v", err)
	}

	// 检查是否大致等待了超时时间
	if elapsed < timeout {
		t.Fatalf("Timeout too short: %v", elapsed)
	}
}

func TestBlockingQueue_ContextCancel(t *testing.T) {
	q := NewQueue[int](WithCapacity(1))

	// 填满队列
	if err := q.TryEnqueue(1); err != nil {
		t.Fatalf("TryEnqueue failed: %v", err)
	}

	// 创建一个可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 启动一个goroutine并尝试入队
	var enqueueErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		enqueueErr = q.Enqueue(ctx, 2) // 应该阻塞
	}()

	// 稍等一会，确保goroutine已经阻塞
	time.Sleep(100 * time.Millisecond)

	// 取消上下文
	cancel()

	// 等待goroutine完成
	wg.Wait()

	// 检查是否返回了正确的错误
	if !errors.Is(enqueueErr, ErrOperationCancelled) {
		t.Fatalf("Expected ErrOperationCancelled, got %v", enqueueErr)
	}
}

func TestBlockingQueue_ConcurrentAccess(t *testing.T) {
	// 设置合理的超时时间
	timeout := 5 * time.Second
	testCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 使用较大队列容量，避免阻塞
	q := NewQueue[int](
		WithCapacity(1000), // 更大的容量
		WithDequeueTimeout(100*time.Millisecond),
	)

	// 并发生产者/消费者计数
	producers := 5
	consumers := 5
	itemsPerProducer := 1000
	totalItems := producers * itemsPerProducer

	// 跟踪已处理的项目
	var consumedCount atomic.Int32
	consumedItems := sync.Map{}

	// 使用原子变量跟踪是否关闭了通知通道
	var doneChannelClosed atomic.Bool

	// 关闭标志，用于通知消费者应该退出了
	done := make(chan struct{})

	// 用于协调生产者完成信号
	producersDone := make(chan struct{})

	// 等待组，用于等待所有生产者和消费者完成
	var wg sync.WaitGroup

	// 启动消费者
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 创建独立的超时上下文
			consumerCtx, consumerCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer consumerCancel()

			for {
				// 首先检查是否应该退出
				select {
				case <-done:
					return
				case <-testCtx.Done():
					t.Logf("Consumer %d: test context canceled", id)
					return
				default:
					// 继续处理
				}

				// 尝试出队
				item, err := q.Dequeue(consumerCtx)
				if err == nil {
					// 成功出队
					if _, loaded := consumedItems.LoadOrStore(item, true); loaded {
						t.Errorf("Item %d consumed more than once", item)
					}
					consumedCount.Add(1)

					// 检查是否消费完所有项目，如果是则提前结束测试
					if int(consumedCount.Load()) >= totalItems {
						if !doneChannelClosed.Swap(true) {
							close(done) // 提前通知其他消费者退出
						}
						return
					}
				} else if errors.Is(err, ErrQueueClosed) {
					// 队列已关闭，退出
					return
				} else if errors.Is(err, ErrOperationTimeout) || errors.Is(err, ErrOperationCancelled) {
					// 超时了，创建新的上下文继续
					consumerCancel()
					consumerCtx, consumerCancel = context.WithTimeout(context.Background(), 100*time.Millisecond)

					// 检查是否应该退出
					select {
					case <-done:
						return
					case <-testCtx.Done():
						return
					case <-producersDone:
						// 生产者已完成，尝试清空队列
						continue
					default:
						// 继续尝试
					}
				}
			}
		}(i)
	}

	// 启动生产者
	var producerWg sync.WaitGroup
	for i := 0; i < producers; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()

			baseValue := producerID * itemsPerProducer
			for j := 0; j < itemsPerProducer; j++ {
				// 检查测试是否已超时
				select {
				case <-testCtx.Done():
					t.Logf("Producer %d: test context canceled", producerID)
					return
				default:
					// 继续执行
				}

				// 入队操作
				item := baseValue + j
				err := q.Enqueue(testCtx, item)
				if err != nil && !errors.Is(err, ErrQueueClosed) {
					t.Logf("Producer %d: enqueue error: %v", producerID, err)
				}
			}
		}(i)
	}

	// 等待所有生产者完成
	go func() {
		producerWg.Wait()
		close(producersDone) // 通知消费者生产者已完成

		// 给消费者一些时间来处理队列中的元素
		time.Sleep(500 * time.Millisecond)

		// 关闭队列，这样消费者会退出
		if err := q.Close(); err != nil {
			t.Logf("Close queue error: %v", err)
		}

		// 确保通知所有消费者退出
		if !doneChannelClosed.Swap(true) {
			close(done)
		}
	}()

	// 设置一个等待超时，避免测试无限等待
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

	// 等待所有goroutine完成
	if !waitWithTimeout(&wg, 3*time.Second) {
		// 不要使用Fatal，以免导致资源泄漏
		t.Error("Timeout waiting for goroutines to complete")
	}

	// 验证所有项目都已被处理
	actualConsumed := int(consumedCount.Load())

	// 记录一下消费数量，用于调试
	t.Logf("Consumed items: %d/%d", actualConsumed, totalItems)

	// 如果实际消费项目数少于总项目数，检查具体哪些项目没有被消费
	if actualConsumed < totalItems {
		missing := []int{}
		for i := 0; i < producers; i++ {
			baseValue := i * itemsPerProducer
			for j := 0; j < itemsPerProducer; j++ {
				item := baseValue + j
				if _, exists := consumedItems.Load(item); !exists {
					missing = append(missing, item)
					if len(missing) < 10 { // 只记录前10个缺失项目
						t.Logf("Missing item: %d", item)
					}
				}
			}
		}
		t.Errorf("Expected %d consumed items, got %d (missing %d items)",
			totalItems, actualConsumed, totalItems-actualConsumed)
	} else if actualConsumed > totalItems {
		t.Errorf("Consumed more items than expected: %d > %d", actualConsumed, totalItems)
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

func TestBlockingQueue_Stats(t *testing.T) {
	q := NewQueue[int](WithCapacity(3))
	ctx := context.Background()

	// 执行一些操作
	q.Enqueue(ctx, 1)
	q.Enqueue(ctx, 2)
	q.Enqueue(ctx, 3)

	// 这会阻塞并超时
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_ = q.Enqueue(ctx2, 4)

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

	if stats.Enqueued != 3 {
		t.Fatalf("Expected 3 enqueued operations, got %d", stats.Enqueued)
	}

	if stats.Dequeued != 1 {
		t.Fatalf("Expected 1 dequeued operation, got %d", stats.Dequeued)
	}

	if stats.Utilization() != float64(2)/float64(3) {
		t.Fatalf("Expected utilization %.2f, got %.2f", float64(2)/float64(3), stats.Utilization())
	}
}

func TestBlockingQueue_Events(t *testing.T) {
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
	q := NewQueue[int](WithCapacity(2), WithEventListener(listener))
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
	if enqueueCalls != 2 {
		t.Fatalf("Expected 2 enqueue events, got %d", enqueueCalls)
	}

	if dequeueCalls != 2 {
		t.Fatalf("Expected 2 dequeue events, got %d", dequeueCalls)
	}

	if fullCalls != 1 {
		t.Fatalf("Expected 1 full event, got %d", fullCalls)
	}

	if emptyCalls != 1 {
		t.Fatalf("Expected 1 empty event, got %d", emptyCalls)
	}

	if closeCalls != 1 {
		t.Fatalf("Expected 1 close event, got %d", closeCalls)
	}

	if errorCalls != 1 {
		t.Fatalf("Expected 1 error event, got %d", errorCalls)
	}
}

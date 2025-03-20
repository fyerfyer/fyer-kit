package workpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBasicTaskExecution 测试基本的任务执行功能
func TestBasicTaskExecution(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(2),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 创建测试任务
	task := TaskFunc(func(ctx context.Context) (interface{}, error) {
		time.Sleep(100 * time.Millisecond) // 模拟工作
		return "success", nil
	})

	// 提交任务
	handle, err := wp.Submit(task)
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 等待结果
	result, err := handle.Result()
	if err != nil {
		t.Fatalf("Task execution failed: %v", err)
	}

	// 验证结果
	if result != "success" {
		t.Errorf("Expected result 'success', got %v", result)
	}
}

// TestTaskPriorities 测试任务优先级
func TestTaskPriorities(t *testing.T) {
	// 创建工作池 - 只用1个工作协程，这样可以确保任务按顺序执行
	wp := New(
		WithFixedPoolSize(1),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 用于追踪执行顺序的变量
	var executionOrder []int
	var mutex sync.Mutex

	// 提交三个具有不同优先级的任务
	createTask := func(id int, priority TaskPriority) TaskFunc {
		return func(ctx context.Context) (interface{}, error) {
			mutex.Lock()
			executionOrder = append(executionOrder, id)
			mutex.Unlock()
			time.Sleep(50 * time.Millisecond) // 模拟工作
			return id, nil
		}
	}

	// 先提交一个低优先级任务
	_, err := wp.Submit(createTask(1, PriorityLow), WithPriority(PriorityLow))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 再提交一个高优先级任务
	_, err = wp.Submit(createTask(2, PriorityHigh), WithPriority(PriorityHigh))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 最后提交一个正常优先级任务
	_, err = wp.Submit(createTask(3, PriorityNormal), WithPriority(PriorityNormal))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 等待所有任务完成
	time.Sleep(500 * time.Millisecond)

	// 验证执行顺序：应该是高优先级 > 正常优先级 > 低优先级
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 tasks to be executed, got %d", len(executionOrder))
	}

	// 高优先级任务应该首先执行
	if executionOrder[0] != 2 {
		t.Errorf("Expected high priority task (ID 2) to execute first, got %d", executionOrder[0])
	}

	// 然后是正常优先级任务
	if executionOrder[1] != 3 {
		t.Errorf("Expected normal priority task (ID 3) to execute second, got %d", executionOrder[1])
	}

	// 最后是低优先级任务
	if executionOrder[2] != 1 {
		t.Errorf("Expected low priority task (ID 1) to execute last, got %d", executionOrder[2])
	}
}

// TestTaskCancellation 测试任务取消功能
func TestTaskCancellation(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(2),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 创建一个长时间运行的任务，检查是否可以取消
	task := TaskFunc(func(ctx context.Context) (interface{}, error) {
		// 检查取消信号
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second): // 正常情况下需要2秒才能完成
			return "completed", nil
		}
	})

	// 提交任务
	handle, err := wp.Submit(task)
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 等待50毫秒后取消任务
	time.Sleep(50 * time.Millisecond)
	err = handle.Cancel()
	if err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	// 验证任务状态已取消
	result, err := handle.Result()
	// 任务可能已经开始执行并收到了取消信号，返回 context.Canceled
	// 或者任务在开始执行前就被取消了，没有错误但状态为 Canceled
	if err != nil {
		// 如果有错误，确保是取消相关的错误
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.Canceled or context.DeadlineExceeded error, got: %v", err)
		}
	}

	if result != nil {
		t.Errorf("Expected nil result after cancellation, got %v", result)
	}

	if handle.Status() != TaskStatusCanceled {
		t.Errorf("Expected status to be Canceled, got %s", handle.Status())
	}
}

// TestTaskTimeout 测试任务超时功能
func TestTaskTimeout(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(2),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 创建一个会超时的任务
	task := TaskFunc(func(ctx context.Context) (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond): // 任务需要500毫秒
			return "completed", nil
		}
	})

	// 提交任务，超时设置为100毫秒
	handle, err := wp.Submit(task, WithTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 等待任务完成
	result, err := handle.Result()

	// 验证任务因超时而失败
	if err == nil {
		t.Errorf("Expected timeout error, got nil")
	}
	if result != nil {
		t.Errorf("Expected nil result after timeout, got %v", result)
	}
}

// TestConcurrentTasks 测试并发任务执行
func TestConcurrentTasks(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(4),
		WithMaxWorkers(8), // 允许扩展到8个工作协程
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 计数器，记录完成的任务数
	var completed int32

	// 提交50个简单任务
	handles := make([]TaskHandle, 0, 50)
	for i := 0; i < 50; i++ {
		task := TaskFunc(func(ctx context.Context) (interface{}, error) {
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&completed, 1)
			return nil, nil
		})

		handle, err := wp.Submit(task)
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
		handles = append(handles, handle)
	}

	// 等待所有任务完成
	for _, h := range handles {
		_, _ = h.Result() // 忽略结果，只关心任务是否完成
	}

	// 验证所有任务都已完成
	if atomic.LoadInt32(&completed) != 50 {
		t.Errorf("Expected 50 completed tasks, got %d", atomic.LoadInt32(&completed))
	}

	// 检查工作池指标
	metrics := wp.GetMetrics()
	if metrics.CompletedTasks != 50 {
		t.Errorf("Expected 50 completed tasks in metrics, got %d", metrics.CompletedTasks)
	}
}

// TestShutdownBehavior 测试工作池关闭行为
func TestShutdownBehavior(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(2),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 创建一个长时间运行的任务
	longTask := TaskFunc(func(ctx context.Context) (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return "done", nil
		}
	})

	// 提交长任务
	handle, err := wp.Submit(longTask)
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 立即开始关闭工作池，但给予足够时间完成
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 关闭工作池
	err = wp.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Failed to shutdown work pool: %v", err)
	}

	// 验证任务仍能完成
	result, err := handle.Result()
	if err != nil {
		t.Errorf("Expected task to complete successfully after shutdown, got error: %v", err)
	}
	if result != "done" {
		t.Errorf("Expected result 'done', got %v", result)
	}

	// 尝试提交新任务，应该失败
	_, err = wp.Submit(TaskFunc(func(ctx context.Context) (interface{}, error) {
		return nil, nil
	}))
	if err == nil {
		t.Error("Expected error when submitting task to stopped pool, got nil")
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	// 创建工作池
	wp := New(
		WithInitialWorkers(2),
		WithLogLevel(LogLevelDebug),
	)

	// 启动工作池
	if err := wp.Start(); err != nil {
		t.Fatalf("Failed to start work pool: %v", err)
	}

	// 确保最后关闭工作池
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = wp.Shutdown(ctx)
	}()

	// 创建一个会返回错误的任务
	errorTask := TaskFunc(func(ctx context.Context) (interface{}, error) {
		return nil, errors.New("planned task failure")
	})

	// 提交任务
	handle, err := wp.Submit(errorTask)
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// 等待结果
	result, err := handle.Result()

	// 验证错误正确传播
	if err == nil {
		t.Error("Expected error from task, got nil")
	}
	if result != nil {
		t.Errorf("Expected nil result with error, got %v", result)
	}
	if handle.Status() != TaskStatusFailed {
		t.Errorf("Expected task status to be Failed, got %s", handle.Status())
	}
}

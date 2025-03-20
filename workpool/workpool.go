package workpool

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// WorkPoolStatus 工作池的状态
type WorkPoolStatus int

const (
	// StatusIdle 空闲状态
	StatusIdle WorkPoolStatus = iota
	// StatusRunning 运行状态
	StatusRunning
	// StatusShuttingDown 正在关闭
	StatusShuttingDown
	// StatusStopped 已停止
	StatusStopped
)

// String 返回工作池状态的字符串表示
func (s WorkPoolStatus) String() string {
	switch s {
	case StatusIdle:
		return "Idle"
	case StatusRunning:
		return "Running"
	case StatusShuttingDown:
		return "ShuttingDown"
	case StatusStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

// WorkPool 管理一组工作协程，处理提交的任务
type WorkPool struct {
	// 工作池配置
	config WorkPoolConfig

	// 任务队列
	taskQueue *priorityQueue

	// 状态控制
	status     WorkPoolStatus
	statusLock sync.RWMutex
	stopCh     chan struct{}

	// 工作协程控制
	workerWg    sync.WaitGroup
	activeCount int32 // 当前活跃（执行任务中）的协程数
	idleCount   int32 // 当前空闲（等待任务）的协程数
	workerCount int32 // 总工作协程数

	// 指标收集
	metrics *Metrics

	// 工作池上下文，用于全局取消
	ctx    context.Context
	cancel context.CancelFunc

	// 伸缩控制
	scaleLock   sync.Mutex
	scaleTimer  *time.Timer
	scaleTicker *time.Ticker
}

// New 创建一个新的工作池
func New(options ...WorkPoolOption) *WorkPool {
	// 加载默认配置
	config := DefaultConfig()

	// 应用选项
	for _, option := range options {
		option(&config)
	}

	// 校正配置
	if config.initialWorkers < config.minWorkers {
		config.initialWorkers = config.minWorkers
	}
	if config.maxWorkers < config.initialWorkers {
		config.maxWorkers = config.initialWorkers
	}

	ctx, cancel := context.WithCancel(context.Background())

	wp := &WorkPool{
		config:    config,
		taskQueue: newPriorityQueue(),
		status:    StatusIdle,
		stopCh:    make(chan struct{}),
		metrics:   newMetrics(),
		ctx:       ctx,
		cancel:    cancel,
	}

	return wp
}

// Start 启动工作池，开始处理任务
func (wp *WorkPool) Start() error {
	wp.statusLock.Lock()
	defer wp.statusLock.Unlock()

	if wp.status == StatusRunning {
		return errors.New("work pool already running")
	}

	if wp.status == StatusShuttingDown {
		return errors.New("work pool is shutting down")
	}

	// 重置状态
	wp.status = StatusRunning
	wp.stopCh = make(chan struct{})

	// 启动初始工作协程
	wp.adjustWorkers(wp.config.initialWorkers)

	// 如果需要自动伸缩，启动伸缩协程
	if wp.config.minWorkers != wp.config.maxWorkers {
		wp.startScaling()
	}

	if wp.config.logLevel >= LogLevelInfo {
		log.Printf("WorkPool started with %d workers (min: %d, max: %d)",
			wp.config.initialWorkers, wp.config.minWorkers, wp.config.maxWorkers)
	}

	return nil
}

// Shutdown 优雅关闭工作池，等待所有任务完成
func (wp *WorkPool) Shutdown(ctx context.Context) error {
	wp.statusLock.Lock()
	if wp.status == StatusStopped || wp.status == StatusShuttingDown {
		wp.statusLock.Unlock()
		return nil
	}
	wp.status = StatusShuttingDown
	wp.statusLock.Unlock()

	if wp.config.logLevel >= LogLevelInfo {
		log.Printf("WorkPool shutting down, waiting for tasks to complete...")
	}

	// 停止伸缩
	if wp.scaleTicker != nil {
		wp.scaleTicker.Stop()
	}

	// 等待队列处理完
	for !wp.taskQueue.IsEmpty() {
		select {
		case <-ctx.Done():
			wp.cancel() // 取消所有正在运行的任务
			if wp.config.logLevel >= LogLevelError {
				log.Printf("WorkPool shutdown context deadline exceeded, %d tasks still in queue",
					wp.taskQueue.Size())
			}
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// 继续等待
		}
	}

	// 通知工作协程退出，但不取消当前运行的任务
	close(wp.stopCh)

	// 等待所有工作协程退出
	doneCh := make(chan struct{})
	go func() {
		wp.workerWg.Wait()
		close(doneCh)
	}()

	// 等待所有工作协程结束或者超时
	select {
	case <-doneCh:
		// 所有工作协程已退出
	case <-ctx.Done():
		// 现在才取消任务，因为我们已经尽力等待它们完成
		wp.cancel()
		if wp.config.logLevel >= LogLevelError {
			log.Printf("WorkPool shutdown context deadline exceeded, %d workers still active",
				atomic.LoadInt32(&wp.workerCount))
		}
		return ctx.Err()
	}

	wp.statusLock.Lock()
	wp.status = StatusStopped
	wp.statusLock.Unlock()

	if wp.config.logLevel >= LogLevelInfo {
		log.Printf("WorkPool shutdown complete")
	}

	return nil
}

// Submit 提交一个任务到工作池
func (wp *WorkPool) Submit(task Task, options ...TaskOption) (TaskHandle, error) {
	wp.statusLock.RLock()
	if wp.status != StatusRunning {
		wp.statusLock.RUnlock()
		return nil, fmt.Errorf("work pool is not running, current status: %s", wp.status)
	}
	wp.statusLock.RUnlock()

	// 合并默认超时选项
	if wp.config.defaultTaskTimeout > 0 {
		hasTimeout := false
		for _, opt := range options {
			tc := &taskConfig{}
			opt(tc)
			if tc.timeout > 0 {
				hasTimeout = true
				break
			}
		}
		if !hasTimeout {
			options = append(options, WithTimeout(wp.config.defaultTaskTimeout))
		}
	}

	// 创建任务句柄
	taskID := uuid.New().String()
	handle := newTaskHandle(taskID, task, wp.ctx, options...)

	// 将任务加入队列
	wp.taskQueue.Enqueue(handle)

	// 更新指标
	wp.metrics.taskSubmitted()

	// 如果工作协程数小于最大值并且有足够的等待任务，考虑添加工作协程
	if atomic.LoadInt32(&wp.workerCount) < int32(wp.config.maxWorkers) {
		queueSize := wp.taskQueue.Size()
		if queueSize > int(atomic.LoadInt32(&wp.idleCount)) {
			wp.tryAddWorker()
		}
	}

	if wp.config.logLevel >= LogLevelDebug {
		log.Printf("Task submitted: %s with priority %d", taskID, handle.config.priority)
	}

	return handle, nil
}

// Status 返回工作池的当前状态
func (wp *WorkPool) Status() WorkPoolStatus {
	wp.statusLock.RLock()
	defer wp.statusLock.RUnlock()
	return wp.status
}

// GetMetrics 返回工作池的指标快照
func (wp *WorkPool) GetMetrics() Metrics {
	return wp.metrics.Snapshot()
}

// WorkerCount 返回当前工作协程数量
func (wp *WorkPool) WorkerCount() int {
	return int(atomic.LoadInt32(&wp.workerCount))
}

// QueueSize 返回当前队列中等待的任务数量
func (wp *WorkPool) QueueSize() int {
	return wp.taskQueue.Size()
}

// TaskCount 返回工作池处理的任务总数
func (wp *WorkPool) TaskCount() uint64 {
	return atomic.LoadUint64(&wp.metrics.TotalTasks)
}

// startScaling 启动自动伸缩协程
func (wp *WorkPool) startScaling() {
	wp.scaleTicker = time.NewTicker(wp.config.scaleInterval)
	go wp.scaleWorkers()
}

// scaleWorkers 根据负载自动调整工作协程数量
func (wp *WorkPool) scaleWorkers() {
	for {
		select {
		case <-wp.scaleTicker.C:
			wp.adjustWorkersBasedOnLoad()
		case <-wp.stopCh:
			return
		}
	}
}

// adjustWorkersBasedOnLoad 根据当前负载调整工作协程数量
func (wp *WorkPool) adjustWorkersBasedOnLoad() {
	wp.scaleLock.Lock()
	defer wp.scaleLock.Unlock()

	// 获取当前状态
	currentWorkers := atomic.LoadInt32(&wp.workerCount)
	activeWorkers := atomic.LoadInt32(&wp.activeCount)
	queueSize := wp.taskQueue.Size()
	queueCapacity := wp.config.queueCapacity

	// 如果队列使用率高，并且有空间增加工作协程，则扩容
	queueUtilization := float64(queueSize) / float64(queueCapacity)
	if queueUtilization >= wp.config.scaleUpThreshold &&
		currentWorkers < int32(wp.config.maxWorkers) {
		// 增加工作协程，但不超过最大值
		newCount := min(int(currentWorkers)+max(1, int(currentWorkers)/4), wp.config.maxWorkers)
		wp.adjustWorkers(newCount)

		if wp.config.logLevel >= LogLevelDebug {
			log.Printf("Scaling up workers from %d to %d (queue: %d/%d, %.1f%%)",
				currentWorkers, newCount, queueSize, queueCapacity, queueUtilization*100)
		}
		return
	}

	// 如果工作协程太多且大部分空闲，则缩容
	idleWorkers := currentWorkers - activeWorkers
	idleRatio := float64(idleWorkers) / float64(currentWorkers)

	if idleRatio >= wp.config.scaleDownThreshold &&
		currentWorkers > int32(wp.config.minWorkers) &&
		queueSize == 0 {
		// 减少工作协程，但不低于最小值
		newCount := max(int(currentWorkers)-max(1, int(currentWorkers)/4), wp.config.minWorkers)
		wp.adjustWorkers(newCount)

		if wp.config.logLevel >= LogLevelDebug {
			log.Printf("Scaling down workers from %d to %d (idle: %d, %.1f%%)",
				currentWorkers, newCount, idleWorkers, idleRatio*100)
		}
	}
}

// adjustWorkers 调整工作协程数量到指定值
func (wp *WorkPool) adjustWorkers(targetCount int) {
	current := int(atomic.LoadInt32(&wp.workerCount))

	// 增加工作协程
	for i := current; i < targetCount; i++ {
		wp.addWorker()
	}

	// 减少工作协程通过stopCh通知完成
	// 多余的工作协程会在下一次尝试获取任务时检测到stopCh关闭并退出
}

// tryAddWorker 尝试添加一个工作协程，如果未达到最大值
func (wp *WorkPool) tryAddWorker() {
	wp.scaleLock.Lock()
	defer wp.scaleLock.Unlock()

	if atomic.LoadInt32(&wp.workerCount) < int32(wp.config.maxWorkers) {
		wp.addWorker()
	}
}

// addWorker 添加一个工作协程
func (wp *WorkPool) addWorker() {
	wp.workerWg.Add(1)
	atomic.AddInt32(&wp.workerCount, 1)
	atomic.AddInt32(&wp.idleCount, 1)

	// 更新指标
	wp.metrics.workerStatusChanged(
		atomic.LoadInt32(&wp.activeCount),
		atomic.LoadInt32(&wp.idleCount),
		atomic.LoadInt32(&wp.workerCount),
	)

	go wp.runWorker()
}

// runWorker 工作协程主循环
func (wp *WorkPool) runWorker() {
	defer func() {
		atomic.AddInt32(&wp.workerCount, -1)

		// 确保活跃或空闲计数正确
		if atomic.LoadInt32(&wp.activeCount) > 0 {
			atomic.AddInt32(&wp.activeCount, -1)
		} else if atomic.LoadInt32(&wp.idleCount) > 0 {
			atomic.AddInt32(&wp.idleCount, -1)
		}

		// 更新指标
		wp.metrics.workerStatusChanged(
			atomic.LoadInt32(&wp.activeCount),
			atomic.LoadInt32(&wp.idleCount),
			atomic.LoadInt32(&wp.workerCount),
		)

		wp.workerWg.Done()
	}()

	for {
		// 检查是否应该退出
		select {
		case <-wp.stopCh:
			return
		case <-wp.ctx.Done():
			return
		default:
			// 继续执行
		}

		// 从队列获取任务
		task := wp.taskQueue.Dequeue()
		if task == nil {
			// 队列为空，等待一会再试
			select {
			case <-time.After(100 * time.Millisecond):
				continue
			case <-wp.stopCh:
				return
			case <-wp.ctx.Done():
				return
			}
		}

		// 更新工作协程状态：从空闲变为活跃
		atomic.AddInt32(&wp.idleCount, -1)
		atomic.AddInt32(&wp.activeCount, 1)

		// 更新指标
		wp.metrics.workerStatusChanged(
			atomic.LoadInt32(&wp.activeCount),
			atomic.LoadInt32(&wp.idleCount),
			atomic.LoadInt32(&wp.workerCount),
		)

		// 设置任务状态为运行中
		submittedTime := time.Now()
		task.setRunning()

		// 记录任务开始执行
		wp.metrics.taskStarted(time.Since(submittedTime))

		// 执行任务
		startTime := time.Now()
		result, err := task.task.Execute(task.ctx)
		processingTime := time.Since(startTime)

		// 任务执行完成，设置结果
		task.setCompleted(result, err)

		// 记录任务完成
		wp.metrics.taskCompleted(processingTime, err == nil)

		if wp.config.logLevel >= LogLevelDebug {
			if err != nil {
				log.Printf("Task %s completed with error in %v: %v",
					task.id, processingTime, err)
			} else {
				log.Printf("Task %s completed successfully in %v",
					task.id, processingTime)
			}
		}

		// 更新工作协程状态：从活跃变为空闲
		atomic.AddInt32(&wp.activeCount, -1)
		atomic.AddInt32(&wp.idleCount, 1)

		// 更新指标
		wp.metrics.workerStatusChanged(
			atomic.LoadInt32(&wp.activeCount),
			atomic.LoadInt32(&wp.idleCount),
			atomic.LoadInt32(&wp.workerCount),
		)
	}
}

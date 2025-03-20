package workpool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// TaskStatus 表示任务的当前状态
type TaskStatus int

const (
	// TaskStatusPending 表示任务正在等待执行
	TaskStatusPending TaskStatus = iota
	// TaskStatusRunning 表示任务正在执行中
	TaskStatusRunning
	// TaskStatusCompleted 表示任务已成功完成
	TaskStatusCompleted
	// TaskStatusFailed 表示任务执行失败
	TaskStatusFailed
	// TaskStatusCanceled 表示任务被取消
	TaskStatusCanceled
)

// String 返回任务状态的字符串表示
func (s TaskStatus) String() string {
	switch s {
	case TaskStatusPending:
		return "Pending"
	case TaskStatusRunning:
		return "Running"
	case TaskStatusCompleted:
		return "Completed"
	case TaskStatusFailed:
		return "Failed"
	case TaskStatusCanceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}

// TaskPriority 表示任务的优先级
type TaskPriority int

const (
	// PriorityLow 低优先级
	PriorityLow TaskPriority = 1
	// PriorityNormal 正常优先级(默认)
	PriorityNormal TaskPriority = 5
	// PriorityHigh 高优先级
	PriorityHigh TaskPriority = 10
)

// Task 是工作池中执行的任务接口
type Task interface {
	// Execute 执行任务并返回结果或错误
	Execute(ctx context.Context) (interface{}, error)
}

// TaskFunc 是一个实现了Task接口的函数类型
type TaskFunc func(ctx context.Context) (interface{}, error)

// Execute 实现Task接口
func (f TaskFunc) Execute(ctx context.Context) (interface{}, error) {
	return f(ctx)
}

// TaskOption 是用于配置任务的函数选项
type TaskOption func(*taskConfig)

// taskConfig 包含任务的配置选项
type taskConfig struct {
	priority TaskPriority
	timeout  time.Duration
}

// WithPriority 设置任务的优先级
func WithPriority(priority TaskPriority) TaskOption {
	return func(tc *taskConfig) {
		tc.priority = priority
	}
}

// WithTimeout 设置任务的超时时间
func WithTimeout(timeout time.Duration) TaskOption {
	return func(tc *taskConfig) {
		tc.timeout = timeout
	}
}

// taskHandle 实现了TaskHandle接口，代表一个已提交的任务
type taskHandle struct {
	id         string
	task       Task
	config     taskConfig
	status     TaskStatus
	result     interface{}
	err        error
	done       chan struct{}
	doneClosed bool // 添加标记以跟踪done通道是否已关闭
	ctx        context.Context
	cancel     context.CancelFunc
	startTime  time.Time
	endTime    time.Time
	mu         sync.RWMutex
}

// TaskHandle 表示已提交到工作池的任务，可用于检查状态和获取结果
type TaskHandle interface {
	// ID 返回任务的唯一标识符
	ID() string
	// Status 返回任务的当前状态
	Status() TaskStatus
	// Result 返回任务的结果，如果任务尚未完成则会阻塞
	Result() (interface{}, error)
	// Cancel 取消任务
	Cancel() error
	// Wait 等待任务完成
	Wait(ctx context.Context) error
}

// newTaskHandle 创建一个新的任务句柄
func newTaskHandle(id string, task Task, ctx context.Context, options ...TaskOption) *taskHandle {
	// 设置默认配置
	config := taskConfig{
		priority: PriorityNormal,
		timeout:  0, // 默认无超时
	}

	// 应用选项
	for _, option := range options {
		option(&config)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	if config.timeout > 0 {
		cancelCtx, cancel = context.WithTimeout(ctx, config.timeout)
	}

	return &taskHandle{
		id:         id,
		task:       task,
		config:     config,
		status:     TaskStatusPending,
		done:       make(chan struct{}),
		doneClosed: false,
		ctx:        cancelCtx,
		cancel:     cancel,
	}
}

// ID 返回任务的唯一标识符
func (h *taskHandle) ID() string {
	return h.id
}

// Status 返回任务的当前状态
func (h *taskHandle) Status() TaskStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// Result 返回任务的结果，如果任务尚未完成则会阻塞
func (h *taskHandle) Result() (interface{}, error) {
	<-h.done
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.result, h.err
}

// Cancel 取消任务
func (h *taskHandle) Cancel() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status == TaskStatusCompleted || h.status == TaskStatusFailed || h.status == TaskStatusCanceled {
		return fmt.Errorf("task already in terminal state: %s", h.status)
	}

	h.status = TaskStatusCanceled
	h.cancel()

	// 如果done通道尚未关闭，则关闭它
	if !h.doneClosed {
		close(h.done)
		h.doneClosed = true
	}

	return nil
}

// Wait 等待任务完成
func (h *taskHandle) Wait(ctx context.Context) error {
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// setRunning 将任务状态设置为运行中
func (h *taskHandle) setRunning() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = TaskStatusRunning
	h.startTime = time.Now()
}

// setCompleted 将任务状态设置为已完成
func (h *taskHandle) setCompleted(result interface{}, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.endTime = time.Now()
	h.result = result
	h.err = err

	// 检查是否是由于取消导致的错误
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		h.status = TaskStatusCanceled
	} else if err != nil {
		h.status = TaskStatusFailed
	} else {
		h.status = TaskStatusCompleted
	}

	// 只有当done通道尚未关闭时才关闭它
	if !h.doneClosed {
		close(h.done)
		h.doneClosed = true
	}
}

// executionTime 返回任务的执行时间
func (h *taskHandle) executionTime() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.startTime.IsZero() {
		return 0
	}

	if h.endTime.IsZero() {
		return time.Since(h.startTime)
	}

	return h.endTime.Sub(h.startTime)
}

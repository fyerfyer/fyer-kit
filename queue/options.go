package queue

import "time"

// Options 定义队列的配置选项
type Options struct {
	// 队列最大容量，0表示无界队列
	Capacity int

	// 入队操作的默认超时时间，0表示永不超时
	EnqueueTimeout time.Duration

	// 出队操作的默认超时时间，0表示永不超时
	DequeueTimeout time.Duration

	// 事件监听器列表
	EventListeners []EventListener
}

// Option 函数类型用于设置队列选项
type Option func(*Options)

// DefaultOptions 返回默认的队列选项
func DefaultOptions() *Options {
	return &Options{
		Capacity:       0, // 默认无界队列
		EnqueueTimeout: 0, // 默认不超时
		DequeueTimeout: 0, // 默认不超时
		EventListeners: nil,
	}
}

// WithCapacity 设置队列容量
func WithCapacity(capacity int) Option {
	return func(o *Options) {
		if capacity < 0 {
			capacity = 0 // 无效容量转为无界队列
		}
		o.Capacity = capacity
	}
}

// WithEnqueueTimeout 设置入队超时
func WithEnqueueTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout < 0 {
			timeout = 0
		}
		o.EnqueueTimeout = timeout
	}
}

// WithDequeueTimeout 设置出队超时
func WithDequeueTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout < 0 {
			timeout = 0
		}
		o.DequeueTimeout = timeout
	}
}

// WithEventListener 添加事件监听器
func WithEventListener(listener EventListener) Option {
	return func(o *Options) {
		if o.EventListeners == nil {
			o.EventListeners = []EventListener{listener}
		} else {
			o.EventListeners = append(o.EventListeners, listener)
		}
	}
}

// WithEventListeners 设置事件监听器列表
func WithEventListeners(listeners []EventListener) Option {
	return func(o *Options) {
		o.EventListeners = listeners
	}
}

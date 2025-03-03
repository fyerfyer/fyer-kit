package pool

import (
	"time"
)

// PoolOptions 定义连接池的配置选项
type PoolOptions struct {
	// InitialSize 是池创建时预先分配的连接数
	InitialSize int

	// MaxIdle 是池中保持空闲状态的最大连接数
	MaxIdle int

	// MaxActive 是池可以分配的最大连接数，包括闲置和使用中的连接
	// 为 0 表示无限制
	MaxActive int

	// MaxIdleTime 是连接保持空闲状态的最长时间
	MaxIdleTime time.Duration

	// MaxLifetime 是连接从创建到关闭的最大生命周期
	MaxLifetime time.Duration

	// IdleCheckFrequency 是检查和清理空闲连接的频率
	IdleCheckFrequency time.Duration

	// WaitTimeout 是在没有可用连接时等待连接可用的最长时间
	// 为 0 表示无限等待
	WaitTimeout time.Duration

	// MaxWaiters 是等待获取连接的最大数量，超过此数量会返回错误
	// 为 0 表示无限制
	MaxWaiters int

	// MinEvictableIdle 是触发驱逐检查器的最小空闲连接数
	MinEvictableIdle int

	// OnCreate 是创建新连接时的回调函数
	OnCreate func(conn Connection) error

	// OnClose 是关闭连接时的回调函数
	OnClose func(conn Connection) error

	// HealthCheck 是连接健康检查函数，返回连接是否健康
	HealthCheck func(conn Connection) bool

	// RetryBackoff 是重试连接的退避策略
	RetryBackoff BackoffStrategy

	// MaxRetries 是获取连接失败时的最大重试次数
	MaxRetries int

	// DialTimeout 是创建新连接时的超时时间
	DialTimeout time.Duration

	// TestOnBorrow 指定是否在从池中获取连接时进行测试
	TestOnBorrow bool

	// TestOnReturn 指定是否在归还连接到池时进行测试
	TestOnReturn bool

	// EventListeners 是连接事件的监听器列表
	EventListeners []EventListener
}

// BackoffStrategy 定义了连接失败时的重试退避策略
type BackoffStrategy func(attempt int) time.Duration

// DefaultOptions 返回默认的连接池选项
func DefaultOptions() *PoolOptions {
	return &PoolOptions{
		InitialSize:        0,
		MaxIdle:            10,
		MaxActive:          100,
		MaxIdleTime:        5 * time.Minute,
		MaxLifetime:        30 * time.Minute,
		IdleCheckFrequency: 1 * time.Minute,
		WaitTimeout:        30 * time.Second,
		MaxWaiters:         0, // 无限制
		MinEvictableIdle:   2,
		RetryBackoff:       ExponentialBackoff,
		MaxRetries:         3,
		DialTimeout:        5 * time.Second,
		TestOnBorrow:       true,
		TestOnReturn:       false,
	}
}

// ExponentialBackoff 实现指数退避策略
func ExponentialBackoff(attempt int) time.Duration {
	return time.Duration(2<<uint(attempt-1)) * time.Millisecond * 100
}

// Option 是用于配置池选项的函数类型
type Option func(*PoolOptions)

// WithInitialSize 设置初始连接数
func WithInitialSize(size int) Option {
	return func(opts *PoolOptions) {
		opts.InitialSize = size
	}
}

// WithMaxIdle 设置最大空闲连接数
func WithMaxIdle(maxIdle int) Option {
	return func(opts *PoolOptions) {
		opts.MaxIdle = maxIdle
	}
}

// WithMaxActive 设置最大活动连接数
func WithMaxActive(maxActive int) Option {
	return func(opts *PoolOptions) {
		opts.MaxActive = maxActive
	}
}

// WithMaxIdleTime 设置最大空闲时间
func WithMaxIdleTime(duration time.Duration) Option {
	return func(opts *PoolOptions) {
		opts.MaxIdleTime = duration
	}
}

// WithMaxLifetime 设置连接最大生命周期
func WithMaxLifetime(duration time.Duration) Option {
	return func(opts *PoolOptions) {
		opts.MaxLifetime = duration
	}
}

// WithIdleCheckFrequency 设置空闲连接检查频率
func WithIdleCheckFrequency(frequency time.Duration) Option {
	return func(opts *PoolOptions) {
		opts.IdleCheckFrequency = frequency
	}
}

// WithWaitTimeout 设置等待连接超时时间
func WithWaitTimeout(timeout time.Duration) Option {
	return func(opts *PoolOptions) {
		opts.WaitTimeout = timeout
	}
}

// WithMaxWaiters 设置最大等待者数量
func WithMaxWaiters(maxWaiters int) Option {
	return func(opts *PoolOptions) {
		opts.MaxWaiters = maxWaiters
	}
}

// WithMinEvictableIdle 设置触发驱逐检查的最小空闲连接数
func WithMinEvictableIdle(minIdle int) Option {
	return func(opts *PoolOptions) {
		opts.MinEvictableIdle = minIdle
	}
}

// WithOnCreate 设置创建连接时的回调函数
func WithOnCreate(callback func(conn Connection) error) Option {
	return func(opts *PoolOptions) {
		opts.OnCreate = callback
	}
}

// WithOnClose 设置关闭连接时的回调函数
func WithOnClose(callback func(conn Connection) error) Option {
	return func(opts *PoolOptions) {
		opts.OnClose = callback
	}
}

// WithHealthCheck 设置连接健康检查函数
func WithHealthCheck(healthCheck func(conn Connection) bool) Option {
	return func(opts *PoolOptions) {
		opts.HealthCheck = healthCheck
	}
}

// WithRetryBackoff 设置重试退避策略
func WithRetryBackoff(strategy BackoffStrategy) Option {
	return func(opts *PoolOptions) {
		opts.RetryBackoff = strategy
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(maxRetries int) Option {
	return func(opts *PoolOptions) {
		opts.MaxRetries = maxRetries
	}
}

// WithDialTimeout 设置连接超时时间
func WithDialTimeout(timeout time.Duration) Option {
	return func(opts *PoolOptions) {
		opts.DialTimeout = timeout
	}
}

// WithTestOnBorrow 设置是否在借用连接时测试连接
func WithTestOnBorrow(test bool) Option {
	return func(opts *PoolOptions) {
		opts.TestOnBorrow = test
	}
}

// WithTestOnReturn 设置是否在归还连接时测试连接
func WithTestOnReturn(test bool) Option {
	return func(opts *PoolOptions) {
		opts.TestOnReturn = test
	}
}

// WithEventListener 添加事件监听器
func WithEventListener(listener EventListener) Option {
	return func(opts *PoolOptions) {
		opts.EventListeners = append(opts.EventListeners, listener)
	}
}

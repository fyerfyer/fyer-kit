package workpool

import (
	"time"
)

// WorkPoolOption 是用于配置工作池的函数选项
type WorkPoolOption func(*WorkPoolConfig)

// WorkPoolConfig 包含工作池的所有配置选项
type WorkPoolConfig struct {
	// 协程控制
	initialWorkers int
	minWorkers     int
	maxWorkers     int

	// 队列控制
	queueCapacity int

	// 任务设置
	defaultTaskTimeout time.Duration

	// 协程伸缩策略
	scaleInterval      time.Duration
	scaleUpThreshold   float64 // 触发扩容的队列负载阈值 (0.0-1.0)
	scaleDownThreshold float64 // 触发缩容的协程空闲率阈值 (0.0-1.0)

	// 日志级别
	logLevel LogLevel
}

// LogLevel 表示日志级别
type LogLevel int

const (
	// LogLevelOff 关闭日志
	LogLevelOff LogLevel = iota
	// LogLevelError 只记录错误
	LogLevelError
	// LogLevelInfo 记录信息和错误
	LogLevelInfo
	// LogLevelDebug 记录所有信息，包括调试信息
	LogLevelDebug
)

// DefaultConfig 返回工作池的默认配置
func DefaultConfig() WorkPoolConfig {
	return WorkPoolConfig{
		initialWorkers:     4,               // 默认初始工作协程数
		minWorkers:         1,               // 最少一个协程
		maxWorkers:         100,             // 最多100个协程
		queueCapacity:      1000,            // 队列容量
		defaultTaskTimeout: 0,               // 默认无超时
		scaleInterval:      time.Second * 5, // 每5秒调整一次
		scaleUpThreshold:   0.7,             // 队列使用率超过70%时扩容
		scaleDownThreshold: 0.3,             // 协程空闲率超过30%时缩容
		logLevel:           LogLevelError,   // 默认只记录错误
	}
}

// WithInitialWorkers 设置工作池初始协程数量
func WithInitialWorkers(count int) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if count > 0 {
			config.initialWorkers = count
		}
	}
}

// WithMinWorkers 设置工作池最小协程数量
func WithMinWorkers(count int) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if count > 0 {
			config.minWorkers = count
		}
	}
}

// WithMaxWorkers 设置工作池最大协程数量
func WithMaxWorkers(count int) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if count > 0 {
			config.maxWorkers = count
		}
	}
}

// WithQueueCapacity 设置任务队列容量
func WithQueueCapacity(capacity int) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if capacity > 0 {
			config.queueCapacity = capacity
		}
	}
}

// WithDefaultTaskTimeout 设置任务的默认超时时间
func WithDefaultTaskTimeout(timeout time.Duration) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if timeout >= 0 {
			config.defaultTaskTimeout = timeout
		}
	}
}

// WithScaleInterval 设置协程伸缩检查的时间间隔
func WithScaleInterval(interval time.Duration) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if interval > 0 {
			config.scaleInterval = interval
		}
	}
}

// WithScaleThresholds 设置扩容和缩容的阈值
func WithScaleThresholds(scaleUp, scaleDown float64) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if scaleUp >= 0 && scaleUp <= 1.0 {
			config.scaleUpThreshold = scaleUp
		}
		if scaleDown >= 0 && scaleDown <= 1.0 {
			config.scaleDownThreshold = scaleDown
		}
	}
}

// WithLogLevel 设置日志级别
func WithLogLevel(level LogLevel) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		config.logLevel = level
	}
}

// WithFixedPoolSize 禁用自动伸缩，使用固定数量的协程
func WithFixedPoolSize(size int) WorkPoolOption {
	return func(config *WorkPoolConfig) {
		if size > 0 {
			config.initialWorkers = size
			config.minWorkers = size
			config.maxWorkers = size
		}
	}
}

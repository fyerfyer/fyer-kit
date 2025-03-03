package connlimit

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	// ErrLimitExceeded 当连接请求超过限流限制时返回
	ErrLimitExceeded = errors.New("connection rate limit exceeded")

	// ErrBurstExceeded 当并发连接请求超过突发限制时返回
	ErrBurstExceeded = errors.New("connection burst limit exceeded")

	// ErrWaitTimeout 当等待连接超过最大等待时间时返回
	ErrWaitTimeout = errors.New("wait for connection timed out")
)

// Limiter 提供连接限流功能
type Limiter interface {
	// Allow 检查是否允许新的连接请求，不等待
	Allow() bool

	// Wait 等待直到允许新的连接请求或上下文取消
	Wait(ctx context.Context) error

	// Reserve 返回需要等待的时间
	Reserve() (time.Duration, bool)

	// Close 清理资源
	Close() error
}

// TokenBucketLimiter 使用令牌桶算法实现连接限流
type TokenBucketLimiter struct {
	limiter     *rate.Limiter
	maxWaitTime time.Duration
}

// TokenBucketOption 是令牌桶限流器的配置选项
type TokenBucketOption func(*TokenBucketLimiter)

// WithMaxWaitTime 设置最大等待时间
func WithMaxWaitTime(d time.Duration) TokenBucketOption {
	return func(l *TokenBucketLimiter) {
		l.maxWaitTime = d
	}
}

// NewTokenBucketLimiter 创建一个新的基于令牌桶算法的限流器
// 参数:
// - rate: 每秒允许的连接请求数
// - burst: 允许的最大突发请求数
func NewTokenBucketLimiter(r float64, burst int, opts ...TokenBucketOption) *TokenBucketLimiter {
	l := &TokenBucketLimiter{
		limiter:     rate.NewLimiter(rate.Limit(r), burst),
		maxWaitTime: 5 * time.Second, // 默认最大等待时间
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// Allow 立即检查是否允许新的连接请求
func (l *TokenBucketLimiter) Allow() bool {
	return l.limiter.Allow()
}

// Wait 等待直到允许新的连接请求或上下文取消
func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	// 如果设置了最大等待时间，使用带超时的上下文
	if l.maxWaitTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.maxWaitTime)
		defer cancel()
	}

	if err := l.limiter.Wait(ctx); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrWaitTimeout
		}
		return err
	}
	return nil
}

// Reserve 返回需要等待的时间
func (l *TokenBucketLimiter) Reserve() (time.Duration, bool) {
	r := l.limiter.Reserve()
	if !r.OK() {
		return 0, false
	}
	return r.Delay(), true
}

// Close 实现 Limiter 接口，清理资源
func (l *TokenBucketLimiter) Close() error {
	return nil
}

// WindowLimiter 使用滑动窗口算法实现连接限流
type WindowLimiter struct {
	mu          sync.Mutex
	windowSize  time.Duration
	maxRequests int
	requests    []time.Time
	maxWaitTime time.Duration
}

// WindowOption 是滑动窗口限流器的配置选项
type WindowOption func(*WindowLimiter)

// WithWindowMaxWaitTime 设置最大等待时间
func WithWindowMaxWaitTime(d time.Duration) WindowOption {
	return func(l *WindowLimiter) {
		l.maxWaitTime = d
	}
}

// NewWindowLimiter 创建一个新的基于滑动窗口算法的限流器
// 参数:
// - windowSize: 滑动窗口的时间大小
// - maxRequests: 窗口内允许的最大请求数
func NewWindowLimiter(windowSize time.Duration, maxRequests int, opts ...WindowOption) *WindowLimiter {
	l := &WindowLimiter{
		windowSize:  windowSize,
		maxRequests: maxRequests,
		requests:    make([]time.Time, 0, maxRequests),
		maxWaitTime: 5 * time.Second, // 默认最大等待时间
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// cleanupOldRequests 清理窗口外的过期请求记录
func (l *WindowLimiter) cleanupOldRequests() {
	now := time.Now()
	windowStart := now.Add(-l.windowSize)

	// 找到第一个在窗口内的请求的索引
	i := 0
	for ; i < len(l.requests); i++ {
		if l.requests[i].After(windowStart) {
			break
		}
	}

	// 移除所有窗口外的请求
	if i > 0 {
		l.requests = l.requests[i:]
	}
}

// Allow 立即检查是否允许新的连接请求
func (l *WindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupOldRequests()

	// 检查是否达到最大请求数
	if len(l.requests) >= l.maxRequests {
		return false
	}

	// 记录新请求
	l.requests = append(l.requests, time.Now())
	return true
}

// Wait 等待直到允许新的连接请求或上下文取消
func (l *WindowLimiter) Wait(ctx context.Context) error {
	// 首先尝试直接获取许可
	if l.Allow() {
		return nil
	}

	// 计算需要等待的时间
	waitTime, ok := l.Reserve()
	if !ok {
		return ErrBurstExceeded
	}

	// 如果设置了最大等待时间，检查是否超过
	if l.maxWaitTime > 0 && waitTime > l.maxWaitTime {
		return ErrWaitTimeout
	}

	// 创建一个定时器等待
	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	select {
	case <-timer.C:
		// 等待结束后再次尝试获取许可
		if l.Allow() {
			return nil
		}
		return ErrLimitExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reserve 返回需要等待的时间
func (l *WindowLimiter) Reserve() (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupOldRequests()

	// 如果请求数未达到上限，无需等待
	if len(l.requests) < l.maxRequests {
		return 0, true
	}

	// 计算最早的请求何时离开窗口
	if len(l.requests) == 0 {
		return 0, true
	}

	oldestRequest := l.requests[0]
	waitTime := oldestRequest.Add(l.windowSize).Sub(time.Now())

	// 如果已经过期，无需等待
	if waitTime <= 0 {
		return 0, true
	}

	return waitTime, true
}

// Close 实现 Limiter 接口，清理资源
func (l *WindowLimiter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清理资源
	l.requests = nil
	return nil
}

// AdaptiveLimiter 自适应限流器，结合多种限流策略
type AdaptiveLimiter struct {
	primary   Limiter
	secondary Limiter
	mode      int // 0: 只使用主限流器, 1: 只有主限流器允许才通过, 2: 任一限流器允许即通过
}

// AdaptiveMode 定义自适应限流器的工作模式
type AdaptiveMode int

const (
	// PrimaryOnly 只使用主限流器
	PrimaryOnly AdaptiveMode = iota

	// AllRequired 所有限流器都必须允许
	AllRequired

	// AnyAllowed 任一限流器允许即通过
	AnyAllowed
)

// NewAdaptiveLimiter 创建一个新的自适应限流器
func NewAdaptiveLimiter(primary, secondary Limiter, mode AdaptiveMode) *AdaptiveLimiter {
	return &AdaptiveLimiter{
		primary:   primary,
		secondary: secondary,
		mode:      int(mode),
	}
}

// Allow 立即检查是否允许新的连接请求
func (l *AdaptiveLimiter) Allow() bool {
	switch l.mode {
	case int(PrimaryOnly):
		return l.primary.Allow()
	case int(AllRequired):
		return l.primary.Allow() && l.secondary.Allow()
	case int(AnyAllowed):
		return l.primary.Allow() || l.secondary.Allow()
	default:
		return l.primary.Allow()
	}
}

// Wait 等待直到允许新的连接请求或上下文取消
func (l *AdaptiveLimiter) Wait(ctx context.Context) error {
	switch l.mode {
	case int(PrimaryOnly):
		return l.primary.Wait(ctx)
	case int(AllRequired):
		if err := l.primary.Wait(ctx); err != nil {
			return err
		}
		return l.secondary.Wait(ctx)
	case int(AnyAllowed):
		// 创建一个通道来接收结果
		errCh := make(chan error, 2)

		// 为两个限流器启动 goroutine
		go func() {
			errCh <- l.primary.Wait(ctx)
		}()

		go func() {
			errCh <- l.secondary.Wait(ctx)
		}()

		// 等待第一个完成的结果
		err1 := <-errCh
		if err1 == nil {
			return nil // 如果其中一个成功，我们就成功了
		}

		// 等待第二个结果
		err2 := <-errCh
		if err2 == nil {
			return nil // 如果第二个成功，我们也成功
		}

		// 两个都失败了，返回主限流器的错误
		return err1
	default:
		return l.primary.Wait(ctx)
	}
}

// Reserve 返回需要等待的时间
func (l *AdaptiveLimiter) Reserve() (time.Duration, bool) {
	switch l.mode {
	case int(PrimaryOnly):
		return l.primary.Reserve()
	case int(AllRequired):
		d1, ok1 := l.primary.Reserve()
		if !ok1 {
			return 0, false
		}
		d2, ok2 := l.secondary.Reserve()
		if !ok2 {
			return 0, false
		}
		if d1 > d2 {
			return d1, true
		}
		return d2, true
	case int(AnyAllowed):
		d1, ok1 := l.primary.Reserve()
		d2, ok2 := l.secondary.Reserve()

		if !ok1 && !ok2 {
			return 0, false
		}

		if !ok1 {
			return d2, true
		}

		if !ok2 {
			return d1, true
		}

		// 两个都可以，返回较小的等待时间
		if d1 < d2 {
			return d1, true
		}
		return d2, true
	default:
		return l.primary.Reserve()
	}
}

// Close 实现 Limiter 接口，清理资源
func (l *AdaptiveLimiter) Close() error {
	err1 := l.primary.Close()
	err2 := l.secondary.Close()

	if err1 != nil {
		return err1
	}
	return err2
}

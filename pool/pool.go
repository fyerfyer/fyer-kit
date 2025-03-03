package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrPoolClosed 表示连接池已关闭
	ErrPoolClosed = errors.New("pool is closed")

	// ErrPoolFull 表示连接池已满
	ErrPoolFull = errors.New("pool is full")

	// ErrConnectionTimeout 表示获取连接超时
	ErrConnectionTimeout = errors.New("connection timeout")

	// ErrTooManyWaiters 表示等待的调用者过多
	ErrTooManyWaiters = errors.New("too many waiters")

	// ErrInvalidConnection 表示连接无效或已损坏
	ErrInvalidConnection = errors.New("invalid connection")
)

// pooledConnection 是Connection接口的封装，用于在池中管理连接
type pooledConnection struct {
	conn      Connection
	pool      *ConnectionPool
	createdAt time.Time
	lastUsed  time.Time
	state     State
}

// Close 实现Connection接口，将连接放回池中而不是直接关闭
func (pc *pooledConnection) Close() error {
	return pc.pool.Put(pc.conn, nil)
}

// Raw 返回底层连接
func (pc *pooledConnection) Raw() interface{} {
	return pc.conn.Raw()
}

// IsAlive 检查连接是否仍然可用
func (pc *pooledConnection) IsAlive() bool {
	// First check if the underlying connection is alive
	alive := pc.conn.IsAlive()

	// If we have a custom health check in the pool, also apply that
	if alive && pc.pool.opts.HealthCheck != nil {
		alive = pc.pool.opts.HealthCheck(pc)
	}

	return alive
}

// ResetState 准备连接以供重用
func (pc *pooledConnection) ResetState() error {
	return pc.conn.ResetState()
}

// ConnectionPool 实现池接口
type ConnectionPool struct {
	// 池配置选项
	opts *PoolOptions

	// 连接工厂，用于创建新连接
	factory ConnectionFactory

	// 空闲连接通道
	idle chan Connection

	// 事件监听器
	eventListeners []EventListener

	// 活动连接计数
	activeCount int32

	// 等待者计数
	waiterCount int32

	// 统计信息
	stats Stats

	// 关闭状态
	closed bool

	// 保护共享状态的互斥锁
	mu sync.RWMutex

	// 维护活动连接的映射
	activeConns sync.Map
}

// NewPool 创建一个新的连接池
func NewPool(factory ConnectionFactory, options ...Option) Pool {
	opts := DefaultOptions()
	for _, option := range options {
		option(opts)
	}

	p := &ConnectionPool{
		opts:           opts,
		factory:        factory,
		idle:           make(chan Connection, opts.MaxIdle),
		eventListeners: opts.EventListeners,
		stats: Stats{
			CreatedAt:   time.Now(),
			MaxIdleTime: opts.MaxIdleTime,
			MaxLifetime: opts.MaxLifetime,
		},
	}

	// 预创建初始连接
	if opts.InitialSize > 0 {
		for i := 0; i < opts.InitialSize; i++ {
			conn, err := p.createConnection(context.Background())
			if err != nil {
				// 记录错误但继续
				continue
			}
			// 使用 Put 来确保连接正确地从活动状态转为空闲状态
			p.Put(conn, nil)
		}
	}

	// 启动后台清理过期连接的 goroutine
	if opts.IdleCheckFrequency > 0 {
		go p.startCleaner(opts.IdleCheckFrequency)
	}

	return p
}

// Get 从池中获取一个连接或创建一个新连接
func (p *ConnectionPool) Get(ctx context.Context) (Connection, error) {
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()

	if closed {
		return nil, ErrPoolClosed
	}

	// 首先尝试从空闲池中获取连接
	select {
	case conn := <-p.idle:
		return p.handleIdleConnection(ctx, conn)
	default:
		// 空闲池为空，检查是否可以创建新连接
		return p.createOrWaitForConnection(ctx)
	}
}

// handleIdleConnection 处理从空闲池获取的连接
func (p *ConnectionPool) handleIdleConnection(ctx context.Context, conn Connection) (Connection, error) {
	pc, ok := conn.(*pooledConnection)
	if !ok {
		// 这不应该发生，但如果发生，关闭连接并创建一个新的
		conn.Close()
		p.updateStats(func(s *Stats) {
			s.Errors++
		})
		return p.createOrWaitForConnection(ctx)
	}

	// 检查是否过期
	now := time.Now()
	if (p.opts.MaxIdleTime > 0 && now.Sub(pc.lastUsed) > p.opts.MaxIdleTime) ||
		(p.opts.MaxLifetime > 0 && now.Sub(pc.createdAt) > p.opts.MaxLifetime) {
		// 连接过期，关闭并创建新连接
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Idle--
			s.Total--
		})
		return p.createOrWaitForConnection(ctx)
	}

	// 检查连接是否还活着，包括自定义健康检查
	if p.opts.TestOnBorrow && !pc.IsAlive() {
		// 连接无效，关闭并创建新连接
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Idle--
			s.Total--
		})
		return p.createOrWaitForConnection(ctx)
	}

	// 更新状态并返回连接
	pc.state = StateInUse
	p.activeConns.Store(pc, struct{}{})
	p.updateStats(func(s *Stats) {
		s.Idle--
		s.Active++
		s.Acquired++
	})
	p.notifyEvent(EventGet, pc)
	return pc, nil
}

// createOrWaitForConnection 创建新连接或等待空闲连接
func (p *ConnectionPool) createOrWaitForConnection(ctx context.Context) (Connection, error) {
	p.mu.RLock()
	// 检查是否达到最大连接限制
	maxActive := p.opts.MaxActive
	current := p.stats.Active + p.stats.Idle
	waiters := p.stats.Waiters
	maxWaiters := p.opts.MaxWaiters
	p.mu.RUnlock()

	//fmt.Printf("DEBUG: maxActive=%d, current=%d, waiters=%d, maxWaiters=%d\n", maxActive, current, waiters, maxWaiters)

	// 如果没有达到最大连接数，尝试创建新连接
	if maxActive == 0 || current < maxActive {
		//fmt.Printf("DEBUG: Creating new connection\n")
		return p.createConnection(ctx)
	}

	// 检查等待者数量是否超过限制
	if maxWaiters > 0 && waiters >= maxWaiters {
		//fmt.Printf("DEBUG: Too many waiters\n")
		return nil, ErrTooManyWaiters
	}

	// 等待空闲连接
	p.updateStats(func(s *Stats) {
		s.Waiters++
	})
	defer p.updateStats(func(s *Stats) {
		s.Waiters--
	})

	// 设置等待超时
	var cancel context.CancelFunc
	if p.opts.WaitTimeout > 0 {
		//fmt.Printf("DEBUG: Setting wait timeout to %v\n", p.opts.WaitTimeout)
		ctx, cancel = context.WithTimeout(ctx, p.opts.WaitTimeout)
		defer cancel()
	}

	// 等待空闲连接或者超时
	select {
	case conn := <-p.idle:
		//fmt.Printf("DEBUG: Got idle connection\n")
		return p.handleIdleConnection(ctx, conn)
	case <-ctx.Done():
		//fmt.Printf("DEBUG: Context done with error: %v\n", ctx.Err())
		p.updateStats(func(s *Stats) {
			s.Timeouts++
		})
		// 尝试最后一次重试或者直接返回超时错误
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && p.opts.MaxRetries > 0 {
			//fmt.Printf("DEBUG: Starting retry attempts, max=%d\n", p.opts.MaxRetries)
			for retry := 0; retry < p.opts.MaxRetries; retry++ {
				//fmt.Printf("DEBUG: Retry attempt %d\n", retry+1)
				// 使用退避策略等待
				if p.opts.RetryBackoff != nil {
					backoffTime := p.opts.RetryBackoff(retry + 1)
					//fmt.Printf("DEBUG: Backing off for %v\n", backoffTime)
					time.Sleep(backoffTime)
				}
				// 再次检查池状态
				p.mu.RLock()
				current := p.stats.Active + p.stats.Idle
				closed := p.closed
				p.mu.RUnlock()

				//fmt.Printf("DEBUG: After backoff: current=%d, maxActive=%d, closed=%v\n", current, maxActive, closed)

				if closed {
					//fmt.Printf("DEBUG: Pool closed during retry\n")
					return nil, ErrPoolClosed
				}

				if maxActive == 0 || current < maxActive {
					// 可以创建新连接了
					//fmt.Printf("DEBUG: Can create new connection during retry\n")
					return p.createConnection(ctx)
				}

				// 尝试从空闲池获取
				select {
				case conn := <-p.idle:
					//fmt.Printf("DEBUG: Got idle connection during retry\n")
					return p.handleIdleConnection(ctx, conn)
				default:
					//fmt.Printf("DEBUG: No idle connections available during retry\n")
					// 继续重试
				}
			}
			//fmt.Printf("DEBUG: All retries failed\n")
		}
		//fmt.Printf("DEBUG: Returning connection timeout error\n")
		return nil, ErrConnectionTimeout
	}
}

// createConnection 创建一个新的连接
func (p *ConnectionPool) createConnection(ctx context.Context) (Connection, error) {
	var dialCtx context.Context
	var cancel context.CancelFunc

	if p.opts.DialTimeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, p.opts.DialTimeout)
		defer cancel()
	} else {
		dialCtx = ctx
	}

	// 创建新连接
	conn, err := p.factory.Create(dialCtx)
	if err != nil {
		p.updateStats(func(s *Stats) {
			s.Errors++
		})
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	// 调用创建后回调
	if p.opts.OnCreate != nil {
		if err := p.opts.OnCreate(conn); err != nil {
			conn.Close()
			p.updateStats(func(s *Stats) {
				s.Errors++
			})
			return nil, fmt.Errorf("OnCreate callback failed: %w", err)
		}
	}

	// 创建池连接包装器
	pc := &pooledConnection{
		conn:      conn,
		pool:      p,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		state:     StateInUse,
	}

	// 更新统计信息
	p.activeConns.Store(pc, struct{}{})
	p.updateStats(func(s *Stats) {
		s.Active++
		s.Total++
		s.Acquired++
	})

	p.notifyEvent(EventNew, pc)
	p.notifyEvent(EventGet, pc)
	return pc, nil
}

// Put 将连接归还到池中
func (p *ConnectionPool) Put(conn Connection, err error) error {
	// 检查连接池是否关闭
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()

	// 检查是否为连接池内连接
	pc, ok := conn.(*pooledConnection)
	if !ok {
		// 如果不是的话，就关闭连接
		if conn != nil {
			conn.Close()
		}
		// 如果这个连接池已经关闭，返回错误
		if closed {
			return ErrPoolClosed
		}
		return ErrInvalidConnection
	}

	// 从活跃连接中把当前节点删掉
	p.activeConns.Delete(pc)

	// 如果连接报错了或者不健康，就把它关掉
	if err != nil || (p.opts.TestOnReturn && !pc.IsAlive()) {
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Total--
		})
		// 如果连接池被关上了，就返回错误
		if closed {
			return ErrPoolClosed
		}
		return nil
	}

	// 重新设置连接状态以让它可以重用
	// 如果设置失败就关闭连接
	if err := pc.ResetState(); err != nil {
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Total--
		})
		// 如果连接池被关上了，就返回错误
		if closed {
			return ErrPoolClosed
		}
		return nil
	}

	// 更新连接状态
	pc.lastUsed = time.Now()
	pc.state = StateIdle

	// 如果连接池关闭，就关闭连接，同时返回错误
	if closed {
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Total--
		})
		return ErrPoolClosed
	}

	// 把连接放回空闲池
	p.mu.RLock()
	idleChannelClosed := p.idle == nil
	p.mu.RUnlock()

	if idleChannelClosed {
		// 空闲池关闭，关闭连接
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Total--
		})
		return ErrPoolClosed
	}

	// 把连接放回空闲通道中
	select {
	case p.idle <- pc:
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Idle++
			s.Released++
		})
		p.notifyEvent(EventPut, pc)
		return nil
	default:
		// 空闲池已满，直接关闭节点
		pc.conn.Close()
		p.notifyEvent(EventClose, pc)
		p.updateStats(func(s *Stats) {
			s.Active--
			s.Total--
		})
		return ErrPoolFull
	}
}

// Shutdown 优雅地关闭池
func (p *ConnectionPool) Shutdown(ctx context.Context) error {
	//fmt.Printf("DEBUG Shutdown: Starting shutdown process\n")

	// 检查是否已经关闭
	p.mu.RLock()
	alreadyClosed := p.closed
	p.mu.RUnlock()

	if alreadyClosed {
		//fmt.Printf("DEBUG Shutdown: Pool already closed\n")
		return ErrPoolClosed
	}

	// 获取池中所有活跃连接
	p.mu.RLock()
	activeCount := p.stats.Active
	//fmt.Printf("DEBUG Shutdown: Pool has %d active connections\n", activeCount)
	p.mu.RUnlock()

	// 把池标记为关闭
	// 池不能提供连接给外部服务使用，但是可以返回节点
	p.mu.Lock()
	p.closed = true
	//fmt.Printf("DEBUG Shutdown: Marked pool as closed\n")
	p.mu.Unlock()

	// 在连接池关闭时，关闭通道并清空通道里面的连接
	//fmt.Printf("DEBUG Shutdown: Closing and draining idle channel\n")

	// 加锁保护对通道的操作
	p.mu.Lock()
	idle := p.idle
	p.idle = nil // Set to nil first to indicate it's been closed
	close(idle)  // Then close it
	p.mu.Unlock()

	// 查找并关闭通道里所有的空闲连接
	for conn := range idle {
		if pc, ok := conn.(*pooledConnection); ok {
			//fmt.Printf("DEBUG Shutdown: Closing idle connection\n")
			pc.conn.Close()
			p.notifyEvent(EventClose, pc)
		}
	}

	if activeCount == 0 {
		//fmt.Printf("DEBUG Shutdown: No active connections, shutdown complete\n")
		return nil
	}

	//fmt.Printf("DEBUG Shutdown: Waiting for %d active connections to be returned\n", activeCount)

	// 等待所有活跃连接被返回或者超时
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			//fmt.Printf("DEBUG Shutdown: Context cancelled/timeout\n")

			// 强制关闭已有的活跃连接
			//fmt.Printf("DEBUG Shutdown: Force closing remaining connections\n")
			p.activeConns.Range(func(k, v interface{}) bool {
				if pc, ok := k.(*pooledConnection); ok {
					pc.conn.Close()
					p.notifyEvent(EventClose, pc)
				}
				return true
			})
			return ctx.Err()

		case <-ticker.C:
			// Check active connections under read lock
			p.mu.RLock()
			activeCount = p.stats.Active
			p.mu.RUnlock()

			//fmt.Printf("DEBUG Shutdown: Tick - %d active connections remaining\n", activeCount)

			if activeCount == 0 {
				//fmt.Printf("DEBUG Shutdown: All connections returned, shutdown complete\n")
				return nil
			}
		}
	}
}

// Stats 返回池的当前统计信息
func (p *ConnectionPool) Stats() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// updateStats 更新统计信息
func (p *ConnectionPool) updateStats(updater func(*Stats)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	updater(&p.stats)
}

// notifyEvent 通知所有事件监听器
func (p *ConnectionPool) notifyEvent(event Event, conn Connection) {
	for _, listener := range p.eventListeners {
		listener.OnEvent(event, conn)
	}
}

// startCleaner 开始清理过期连接的后台任务
func (p *ConnectionPool) startCleaner(frequency time.Duration) {
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()

		if closed {
			return
		}

		p.cleanIdleConnections()
	}
}

// cleanIdleConnections 清理过期的空闲连接
func (p *ConnectionPool) cleanIdleConnections() {
	var expiredConns []Connection
	idleCount := len(p.idle)

	// 如果空闲连接数低于最小清理阈值，跳过
	if idleCount <= p.opts.MinEvictableIdle {
		return
	}

	// 最多检查当前所有空闲连接
	for i := 0; i < idleCount; i++ {
		select {
		case conn := <-p.idle:
			pc, ok := conn.(*pooledConnection)
			if !ok {
				continue
			}

			now := time.Now()
			expired := (p.opts.MaxIdleTime > 0 && now.Sub(pc.lastUsed) > p.opts.MaxIdleTime) ||
				(p.opts.MaxLifetime > 0 && now.Sub(pc.createdAt) > p.opts.MaxLifetime)

			if expired {
				// 连接过期，加入待关闭列表
				expiredConns = append(expiredConns, pc)
				p.updateStats(func(s *Stats) {
					s.Idle--
					s.Total--
				})
			} else {
				// 连接未过期，放回池中
				select {
				case p.idle <- pc:
					// 成功放回
				default:
					// 池已满，关闭连接
					expiredConns = append(expiredConns, pc)
					p.updateStats(func(s *Stats) {
						s.Idle--
						s.Total--
					})
				}
			}
		default:
			// 没有更多的空闲连接
			break
		}
	}

	// 关闭所有过期连接
	for _, conn := range expiredConns {
		if pc, ok := conn.(*pooledConnection); ok {
			pc.conn.Close()
			p.notifyEvent(EventClose, pc)
		}
	}
}

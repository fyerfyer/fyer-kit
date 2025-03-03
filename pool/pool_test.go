package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnection 实现 Connection 接口，用于测试
type mockConnection struct {
	id        int
	closed    bool
	alive     bool
	resetErr  error
	closeErr  error
	mu        sync.RWMutex
	resetHook func()
}

func (m *mockConnection) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("connection already closed")
	}
	if m.closeErr != nil {
		return m.closeErr
	}
	m.closed = true
	return nil
}

func (m *mockConnection) Raw() interface{} {
	return m.id
}

func (m *mockConnection) IsAlive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.alive && !m.closed
}

func (m *mockConnection) ResetState() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resetHook != nil {
		m.resetHook()
	}
	return m.resetErr
}

// mockFactory 实现 ConnectionFactory 接口，用于测试
type mockFactory struct {
	counter         int32
	createErr       error
	createDelay     time.Duration
	createHook      func(int)
	maxConnections  int32
	createdConns    map[int]*mockConnection
	mu              sync.Mutex
	failOnConnCount int32
}

func (f *mockFactory) Create(ctx context.Context) (Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 模拟创建延迟
	if f.createDelay > 0 {
		select {
		case <-time.After(f.createDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 模拟创建错误
	if f.createErr != nil {
		return nil, f.createErr
	}

	// 模拟连接数限制
	count := atomic.AddInt32(&f.counter, 1)
	if f.maxConnections > 0 && count > f.maxConnections {
		atomic.AddInt32(&f.counter, -1)
		return nil, errors.New("max connections reached")
	}

	// 模拟特定连接失败
	if f.failOnConnCount > 0 && count%f.failOnConnCount == 0 {
		atomic.AddInt32(&f.counter, -1)
		return nil, errors.New("simulated connection failure")
	}

	conn := &mockConnection{
		id:    int(count),
		alive: true,
	}

	if f.createdConns == nil {
		f.createdConns = make(map[int]*mockConnection)
	}
	f.createdConns[int(count)] = conn

	// 调用钩子（如果有）
	if f.createHook != nil {
		f.createHook(int(count))
	}

	return conn, nil
}

// mockEventListener 实现 EventListener 接口，用于测试
type mockEventListener struct {
	events []struct {
		event Event
		conn  Connection
	}
	mu sync.Mutex
}

func (l *mockEventListener) OnEvent(event Event, conn Connection) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, struct {
		event Event
		conn  Connection
	}{event, conn})
}

// 基本的连接池功能测试
func TestConnectionPool_BasicOperations(t *testing.T) {
	factory := &mockFactory{}
	pool := NewPool(factory)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 检查连接ID
	assert.Equal(t, 1, conn.Raw())

	// 返回连接到池中
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 再次获取连接（应该复用刚才返回的连接）
	conn2, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn2)
	assert.Equal(t, 1, conn2.Raw())

	// 关闭池
	// 先把取出来的连接放回去
	err = pool.Put(conn, nil)
	require.NoError(t, err)
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试并发操作
func TestConnectionPool_Concurrency(t *testing.T) {
	factory := &mockFactory{
		maxConnections: 10,
	}

	pool := NewPool(factory, WithMaxActive(10), WithMaxIdle(5))

	// 并发获取和返回连接
	var wg sync.WaitGroup
	concurrentClients := 100
	iterations := 10
	wg.Add(concurrentClients)

	// 记录获取的所有连接ID
	var connIDs sync.Map
	var errCount int32

	for i := 0; i < concurrentClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				ctx := context.Background()
				conn, err := pool.Get(ctx)
				if err != nil {
					atomic.AddInt32(&errCount, 1)
					continue
				}

				// 记录连接ID
				id := conn.Raw().(int)
				connIDs.Store(id, true)

				// 模拟使用连接
				time.Sleep(time.Millisecond * 10)

				// 返回连接
				err = pool.Put(conn, nil)
				if err != nil {
					atomic.AddInt32(&errCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	// 检查连接池状态
	stats := pool.Stats()
	t.Logf("Stats: Active=%d, Idle=%d, Total=%d, Acquired=%d, Released=%d",
		stats.Active, stats.Idle, stats.Total, stats.Acquired, stats.Released)

	// 池中应该没有活动连接
	assert.Equal(t, 0, stats.Active)
	// 池中应该有一些空闲连接
	assert.LessOrEqual(t, stats.Idle, 5) // 最大空闲连接数为5
	// 总连接数应该小于等于最大连接数
	assert.LessOrEqual(t, stats.Total, 10)

	// 检查实际创建的连接数
	var idCount int
	connIDs.Range(func(_, _ interface{}) bool {
		idCount++
		return true
	})
	t.Logf("Unique connection IDs: %d", idCount)

	// 应该复用连接，所以唯一ID数应该小于等于最大连接数
	assert.LessOrEqual(t, idCount, 10)

	// 关闭池
	err := pool.Shutdown(context.Background())
	require.NoError(t, err)
}

// 测试连接超时
func TestConnectionPool_Timeout(t *testing.T) {
	factory := &mockFactory{
		createDelay: 200 * time.Millisecond, // 创建一个连接需要200ms
	}

	pool := NewPool(factory, WithWaitTimeout(100*time.Millisecond), WithMaxActive(1))

	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 在第一个连接正在使用时尝试取出第二个连接
	// 这应该出发超时错误
	_, err = pool.Get(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrConnectionTimeout, err)

	err = pool.Put(conn, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// 现在应该可以拿一个新的连接了
	conn, err = pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	err = pool.Put(conn, nil)
	require.NoError(t, err)
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试连接最大生命周期
func TestConnectionPool_MaxLifetime(t *testing.T) {
	factory := &mockFactory{}

	// 设置连接最大生命周期为200ms
	pool := NewPool(factory, WithMaxLifetime(200*time.Millisecond))

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)
	id1 := conn.Raw().(int)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 等待超过最大生命周期
	time.Sleep(300 * time.Millisecond)

	// 再次获取连接（应该得到一个新连接）
	conn, err = pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)
	id2 := conn.Raw().(int)

	// 连接ID应该不同
	assert.NotEqual(t, id1, id2)

	// 关闭池
	pool.Put(conn, nil)
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试连接最大空闲时间
func TestConnectionPool_MaxIdleTime(t *testing.T) {
	factory := &mockFactory{}

	// 设置连接最大空闲时间为200ms
	pool := NewPool(factory, WithMaxIdleTime(200*time.Millisecond))

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)
	id1 := conn.Raw().(int)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 等待超过最大空闲时间
	time.Sleep(300 * time.Millisecond)

	// 再次获取连接（应该得到一个新连接）
	conn, err = pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)
	id2 := conn.Raw().(int)

	// 连接ID应该不同
	assert.NotEqual(t, id1, id2)

	// 关闭池
	pool.Put(conn, nil)
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试事件监听
func TestConnectionPool_EventListener(t *testing.T) {
	factory := &mockFactory{}
	listener := &mockEventListener{}

	// 添加事件监听器
	pool := NewPool(factory, WithEventListener(listener))

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 检查事件
	assert.Equal(t, 3, len(listener.events)) // EventNew, EventGet, EventPut
	assert.Equal(t, EventNew, listener.events[0].event)
	assert.Equal(t, EventGet, listener.events[1].event)
	assert.Equal(t, EventPut, listener.events[2].event)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)

	// 应该有额外的关闭事件
	assert.Equal(t, 4, len(listener.events)) // +EventClose
	assert.Equal(t, EventClose, listener.events[3].event)
}

// 测试连接健康检查
func TestConnectionPool_HealthCheck(t *testing.T) {
	factory := &mockFactory{}

	// 自定义健康检查函数，只允许偶数ID的连接
	healthCheck := func(conn Connection) bool {
		id := conn.Raw().(int)
		return id%2 == 0
	}

	// 设置健康检查
	pool := NewPool(factory,
		WithTestOnBorrow(true),
		WithHealthCheck(healthCheck))

	// 尝试获取连接，直到得到一个偶数ID的连接
	ctx := context.Background()
	var conn Connection
	var id int
	var err error

	// 我们可能需要多次尝试，因为连接ID是递增的
	for i := 0; i < 10; i++ {
		conn, err = pool.Get(ctx)
		require.NoError(t, err)
		require.NotNil(t, conn)

		id = conn.Raw().(int)
		if id%2 == 0 {
			break // 找到了偶数ID的连接
		}

		// 返回连接（会被销毁，因为健康检查失败）
		err = pool.Put(conn, nil)
		require.NoError(t, err)
	}

	// 确保我们得到的是偶数ID的连接
	assert.Equal(t, 0, id%2)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试处理连接错误
func TestConnectionPool_ConnectionError(t *testing.T) {
	factory := &mockFactory{}

	pool := NewPool(factory)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 模拟连接错误
	mockConn := conn.(*pooledConnection).conn.(*mockConnection)
	mockConn.alive = false

	// 返回连接（应该被销毁而不是放回池中）
	err = pool.Put(conn, errors.New("connection error"))
	require.NoError(t, err)

	// 检查池状态
	stats := pool.Stats()
	assert.Equal(t, 0, stats.Idle)
	assert.Equal(t, 0, stats.Active)
	assert.Equal(t, 0, stats.Total)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试初始连接
func TestConnectionPool_InitialConnections(t *testing.T) {
	factory := &mockFactory{}

	// 设置初始连接数为5
	pool := NewPool(factory, WithInitialSize(5))

	// 检查池状态
	stats := pool.Stats()
	assert.Equal(t, 0, stats.Active)
	assert.Equal(t, 5, stats.Idle)
	assert.Equal(t, 5, stats.Total)

	// 关闭池之前，先确认是否有正在使用的连接导致超时
	// 获取所有连接并立即返回，确保没有泄漏
	var conns []Connection
	ctx := context.Background()
	for i := 0; i < stats.Idle; i++ {
		conn, err := pool.Get(ctx)
		if err != nil {
			break
		}
		conns = append(conns, conn)
	}

	// 返回所有获取的连接
	for _, conn := range conns {
		pool.Put(conn, nil)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := pool.Shutdown(shutdownCtx)
	require.NoError(t, err)
}

// TestConnectionPool_CustomBackoff tests a custom backoff strategy
func TestConnectionPool_CustomBackoff(t *testing.T) {
	attempts := []int{}
	customBackoff := func(attempt int) time.Duration {
		t.Logf("DEBUG: CustomBackoff called with attempt=%d", attempt)
		attempts = append(attempts, attempt)
		return time.Millisecond * 50
	}

	factory := &mockFactory{}

	// 创建一个连接池，设置最大活动连接数为1来强迫等待
	pool := NewPool(factory,
		WithMaxActive(1),
		WithMaxRetries(3),
		WithRetryBackoff(customBackoff),
		WithWaitTimeout(100*time.Millisecond)) // 设置较短等待时长

	// 获取第一个连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 尝试获取另一个连接 - 这应该会触发等待超时
	_, err = pool.Get(ctx)
	assert.Equal(t, ErrConnectionTimeout, err)

	// 检查是否调用了回退策略
	assert.Equal(t, []int{1, 2, 3}, attempts)

	err = pool.Put(conn, nil)
	require.NoError(t, err)
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试池被关闭时的行为
func TestConnectionPool_ClosedPool(t *testing.T) {
	factory := &mockFactory{}
	pool := NewPool(factory)

	// 关闭池
	ctx := context.Background()
	err := pool.Shutdown(ctx)
	require.NoError(t, err)

	// 尝试从已关闭的池获取连接
	_, err = pool.Get(ctx)
	assert.Equal(t, ErrPoolClosed, err)

	// 尝试向已关闭的池返回连接
	conn := &mockConnection{id: 999, alive: true}
	err = pool.Put(conn, nil)
	assert.Equal(t, ErrPoolClosed, err)
}

// 测试清理过期连接
func TestConnectionPool_CleanExpiredConnections(t *testing.T) {
	factory := &mockFactory{}

	// 设置较短的最大空闲时间和检查频率
	pool := NewPool(factory,
		WithMaxIdleTime(100*time.Millisecond),
		WithIdleCheckFrequency(50*time.Millisecond))

	// 获取多个连接
	ctx := context.Background()
	var conns []Connection
	for i := 0; i < 5; i++ {
		conn, err := pool.Get(ctx)
		require.NoError(t, err)
		conns = append(conns, conn)
	}

	// 返回所有连接
	for _, conn := range conns {
		err := pool.Put(conn, nil)
		require.NoError(t, err)
	}

	// 初始状态应该有5个空闲连接
	stats := pool.Stats()
	assert.Equal(t, 5, stats.Idle)

	// 等待清理周期执行（空闲时间 + 检查频率）
	time.Sleep(200 * time.Millisecond)

	// 检查池状态，应该没有空闲连接了
	stats = pool.Stats()
	assert.Equal(t, 0, stats.Idle)

	// 关闭池
	err := pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试优雅关闭
func TestConnectionPool_GracefulShutdown(t *testing.T) {
	factory := &mockFactory{}
	pool := NewPool(factory)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)

	// 启动shutdown过程
	shutdownDone := make(chan error, 1)

	go func() {
		// 设置合理的关闭超时
		shutdownCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		// 延迟一点启动关闭过程，确保主测试goroutine已经准备好
		time.Sleep(50 * time.Millisecond)

		// 关闭池 - 应该等待连接返回
		shutdownDone <- pool.Shutdown(shutdownCtx)
	}()

	// 主goroutine等待一下再返回连接，确保shutdown已经开始
	time.Sleep(200 * time.Millisecond)

	// 返回连接 - 可能会返回ErrPoolClosed，但不应该panic
	putErr := pool.Put(conn, nil)
	if putErr != nil && putErr != ErrPoolClosed {
		t.Errorf("Unexpected error returning connection: %v", putErr)
	}

	// 等待关闭完成
	err = <-shutdownDone
	require.NoError(t, err, "Shutdown should complete without error")
}

// 测试关闭超时
func TestConnectionPool_ShutdownTimeout(t *testing.T) {
	factory := &mockFactory{}
	pool := NewPool(factory)

	// 获取连接但不返回
	ctx := context.Background()
	_, err := pool.Get(ctx)
	require.NoError(t, err)

	// 设置较短的关闭超时
	shutdownCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// 关闭应该超时
	startTime := time.Now()
	err = pool.Shutdown(shutdownCtx)
	elapsed := time.Since(startTime)

	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
}

// 测试错误传播
func TestConnectionPool_ErrorPropagation(t *testing.T) {
	customErr := errors.New("custom error")
	factory := &mockFactory{
		createErr: customErr,
	}

	pool := NewPool(factory)

	// 尝试获取连接，应该返回工厂的错误
	ctx := context.Background()
	_, err := pool.Get(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), customErr.Error())

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试完整的连接池配置
func TestConnectionPool_FullConfiguration(t *testing.T) {
	factory := &mockFactory{}
	listener := &mockEventListener{}

	pool := NewPool(factory,
		WithInitialSize(2),
		WithMaxIdle(5),
		WithMaxActive(10),
		WithMaxIdleTime(time.Minute),
		WithMaxLifetime(time.Hour),
		WithIdleCheckFrequency(time.Minute),
		WithWaitTimeout(time.Second*5),
		WithMaxWaiters(100),
		WithDialTimeout(time.Second*3),
		WithTestOnBorrow(true),
		WithTestOnReturn(true),
		WithEventListener(listener),
		WithRetryBackoff(ExponentialBackoff),
		WithMaxRetries(3),
	)

	// 检查初始状态
	stats := pool.Stats()
	assert.Equal(t, 0, stats.Active)
	assert.Equal(t, 2, stats.Idle)
	assert.Equal(t, 2, stats.Total)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 检查事件
	assert.GreaterOrEqual(t, len(listener.events), 3)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试使用自定义连接
func TestConnectionPool_CustomConnection(t *testing.T) {
	// 自定义连接实现
	type customConn struct {
		mockConnection
		customData string
	}

	// 自定义连接创建工厂
	customFactory := &mockFactory{
		createHook: func(id int) {
			// 可以在这里执行自定义初始化
		},
	}

	pool := NewPool(customFactory)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 检查是否可以将连接转换回原始类型
	rawConn := conn.Raw().(int)
	assert.Equal(t, 1, rawConn)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试连接验证
func TestConnectionPool_ConnectionValidation(t *testing.T) {
	var resetCalled bool
	factory := &mockFactory{}
	factory.createHook = func(id int) {
		conn := factory.createdConns[id]
		conn.resetHook = func() {
			resetCalled = true
		}
	}

	pool := NewPool(factory)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 检查 ResetState 是否被调用
	assert.True(t, resetCalled)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

// 测试统计信息
func TestConnectionPool_Stats(t *testing.T) {
	factory := &mockFactory{}
	pool := NewPool(factory)

	// 检查初始统计
	stats := pool.Stats()
	assert.Equal(t, 0, stats.Active)
	assert.Equal(t, 0, stats.Idle)
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0, stats.Waiters)
	assert.Equal(t, int64(0), stats.Acquired)
	assert.Equal(t, int64(0), stats.Released)

	// 获取连接
	ctx := context.Background()
	conn, err := pool.Get(ctx)
	require.NoError(t, err)

	// 检查更新后的统计
	stats = pool.Stats()
	assert.Equal(t, 1, stats.Active)
	assert.Equal(t, 0, stats.Idle)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, int64(1), stats.Acquired)
	assert.Equal(t, int64(0), stats.Released)

	// 返回连接
	err = pool.Put(conn, nil)
	require.NoError(t, err)

	// 再次检查统计
	stats = pool.Stats()
	assert.Equal(t, 0, stats.Active)
	assert.Equal(t, 1, stats.Idle)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, int64(1), stats.Acquired)
	assert.Equal(t, int64(1), stats.Released)

	// 关闭池
	err = pool.Shutdown(ctx)
	require.NoError(t, err)
}

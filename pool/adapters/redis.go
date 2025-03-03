package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/fyerfyer/fyer-kit/pool"
	"github.com/go-redis/redis/v8"
)

// RedisConnection 实现 Connection 接口，包装 Redis 客户端
type RedisConnection struct {
	client     *redis.Client
	lastUsed   time.Time
	mutex      sync.RWMutex
	closed     bool
	healthFunc func(*redis.Client) bool
}

// Close 实现 Connection 接口的关闭方法
func (c *RedisConnection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	// 我们不实际关闭连接，而是将其标记为关闭并返回到池中
	c.closed = true
	return nil
}

// Raw 返回底层的 Redis 客户端
func (c *RedisConnection) Raw() interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.client
}

// IsAlive 检查 Redis 连接是否仍然可用
func (c *RedisConnection) IsAlive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.closed {
		return false
	}

	// 如果有自定义健康检查函数，使用它
	if c.healthFunc != nil {
		return c.healthFunc(c.client)
	}

	// 默认健康检查：使用 PING 命令
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, err := c.client.Ping(ctx).Result()
	return err == nil && result == "PONG"
}

// ResetState 为重用做准备
func (c *RedisConnection) ResetState() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastUsed = time.Now()
	c.closed = false
	return nil
}

// RedisConfig 定义 Redis 连接配置
type RedisConfig struct {
	// 连接设置
	Addr     string
	Username string
	Password string
	DB       int

	// 连接池设置
	PoolSize     int
	MinIdleConns int
	MaxConnAge   time.Duration

	// 超时设置
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration

	// TLS 配置
	TLSConfig *redis.Options

	// 自定义健康检查函数
	HealthCheck func(*redis.Client) bool
}

// DefaultRedisConfig 返回默认的 Redis 配置
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:         "localhost:6379",
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
		MaxConnAge:   0, // 不限制连接寿命
	}
}

// RedisConnectionFactory 用于创建 Redis 连接
type RedisConnectionFactory struct {
	config *RedisConfig
}

// NewRedisConnectionFactory 创建一个新的 Redis 连接工厂
func NewRedisConnectionFactory(config *RedisConfig) *RedisConnectionFactory {
	if config == nil {
		config = DefaultRedisConfig()
	}

	return &RedisConnectionFactory{
		config: config,
	}
}

// Create 实现 ConnectionFactory 接口，创建一个新的 Redis 连接
func (f *RedisConnectionFactory) Create(ctx context.Context) (pool.Connection, error) {
	options := &redis.Options{
		Addr:         f.config.Addr,
		Username:     f.config.Username,
		Password:     f.config.Password,
		DB:           f.config.DB,
		DialTimeout:  f.config.DialTimeout,
		ReadTimeout:  f.config.ReadTimeout,
		WriteTimeout: f.config.WriteTimeout,
		PoolSize:     f.config.PoolSize,
		MinIdleConns: f.config.MinIdleConns,
		MaxConnAge:   f.config.MaxConnAge,
		PoolTimeout:  f.config.PoolTimeout,
	}

	// 创建 Redis 客户端
	client := redis.NewClient(options)

	// 立即验证连接
	pingCtx, cancel := context.WithTimeout(ctx, f.config.DialTimeout)
	defer cancel()

	if _, err := client.Ping(pingCtx).Result(); err != nil {
		client.Close()
		return nil, err
	}

	conn := &RedisConnection{
		client:     client,
		lastUsed:   time.Now(),
		healthFunc: f.config.HealthCheck,
	}

	return conn, nil
}

// WithRedisClient 创建包装已有 Redis 客户端的连接工厂
func WithRedisClient(client *redis.Client, healthCheck func(*redis.Client) bool) pool.ConnectionFactory {
	return &preCreatedRedisFactory{
		client:     client,
		healthFunc: healthCheck,
	}
}

// preCreatedRedisFactory 用于包装已存在的 Redis 客户端
type preCreatedRedisFactory struct {
	client     *redis.Client
	healthFunc func(*redis.Client) bool
}

// Create 直接返回包装已有客户端的连接
func (f *preCreatedRedisFactory) Create(ctx context.Context) (pool.Connection, error) {
	// 简单验证连接
	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	if _, err := f.client.Ping(pingCtx).Result(); err != nil {
		return nil, err
	}

	conn := &RedisConnection{
		client:     f.client,
		lastUsed:   time.Now(),
		healthFunc: f.healthFunc,
	}

	return conn, nil
}

// RedisPoolHelper Redis 连接池助手
type RedisPoolHelper struct {
	pool pool.Pool
}

// NewRedisPoolHelper 创建一个新的 Redis 连接池助手
func NewRedisPoolHelper(p pool.Pool) *RedisPoolHelper {
	return &RedisPoolHelper{
		pool: p,
	}
}

// Execute 执行 Redis 命令并自动管理连接
func (h *RedisPoolHelper) Execute(ctx context.Context, fn func(*redis.Client) (interface{}, error)) (interface{}, error) {
	// 从池中获取连接
	conn, err := h.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	// 确保连接返回池中
	defer conn.Close()

	// 获取底层 Redis 客户端
	client, ok := conn.Raw().(*redis.Client)
	if !ok {
		return nil, h.pool.Put(conn, err)
	}

	// 执行用户提供的函数
	result, err := fn(client)

	// 处理可能的错误
	if err != nil {
		return nil, h.pool.Put(conn, err)
	}

	// 正常返回连接到池中
	if err = h.pool.Put(conn, nil); err != nil {
		return result, err
	}

	return result, nil
}

// ExecuteCmd 执行 Redis 命令，返回 Redis 命令结果
func (h *RedisPoolHelper) ExecuteCmd(ctx context.Context, cmd func(*redis.Client) *redis.Cmd) (*redis.Cmd, error) {
	var resultCmd *redis.Cmd

	result, err := h.Execute(ctx, func(client *redis.Client) (interface{}, error) {
		resultCmd = cmd(client)
		return resultCmd, resultCmd.Err()
	})

	if err != nil {
		return nil, err
	}

	return result.(*redis.Cmd), nil
}

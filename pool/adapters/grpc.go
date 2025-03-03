package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/fyerfyer/fyer-kit/pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// GRPCConnection 实现 Connection 接口，包装 gRPC 客户端连接
type GRPCConnection struct {
	conn      *grpc.ClientConn
	target    string
	lastUsed  time.Time
	mutex     sync.RWMutex
	closed    bool
	healthCfg *GRPCHealthConfig
}

// GRPCHealthConfig 定义 gRPC 连接健康检查配置
type GRPCHealthConfig struct {
	// 允许的连接状态 (默认只允许 READY 和 IDLE)
	AllowedStates map[connectivity.State]bool
	// 自定义健康检查函数
	HealthCheck func(*grpc.ClientConn) bool
}

// Close 实现 Connection 接口的关闭方法
func (c *GRPCConnection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.conn.Close()
}

// Raw 返回底层的 gRPC 客户端连接
func (c *GRPCConnection) Raw() interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.conn
}

// IsAlive 检查 gRPC 连接是否仍然可用
func (c *GRPCConnection) IsAlive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.closed {
		return false
	}

	// 如果有自定义健康检查函数，使用它
	if c.healthCfg != nil && c.healthCfg.HealthCheck != nil {
		return c.healthCfg.HealthCheck(c.conn)
	}

	// 默认使用连接状态检查
	state := c.conn.GetState()
	if c.healthCfg != nil && c.healthCfg.AllowedStates != nil {
		return c.healthCfg.AllowedStates[state]
	}

	// 默认只接受 READY 和 IDLE 状态
	return state == connectivity.Ready || state == connectivity.Idle
}

// ResetState 准备连接以供重用
func (c *GRPCConnection) ResetState() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastUsed = time.Now()
	c.closed = false

	// 如果连接不在 READY 或 IDLE 状态，尝试重置
	state := c.conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		c.conn.ResetConnectBackoff()
	}

	return nil
}

// GRPCClientConfig 定义 gRPC 客户端连接的配置
type GRPCClientConfig struct {
	// 连接目标地址
	Target string

	// 凭证选项
	Secure               bool
	TransportCredentials credentials.TransportCredentials
	PerRPCCredentials    credentials.PerRPCCredentials
	InsecureSkipVerify   bool
	ServerNameOverride   string

	// 拦截器
	UnaryInterceptors  []grpc.UnaryClientInterceptor
	StreamInterceptors []grpc.StreamClientInterceptor

	// 连接选项
	Block                 bool
	Timeout               time.Duration
	MaxSendMsgSize        int
	MaxRecvMsgSize        int
	MaxHeaderListSize     *uint32
	DisableRetry          bool
	DisableServiceConfig  bool
	EnableRetry           bool
	MaxRetryRPCBufferSize int
	UserAgent             string

	// 保活选项
	KeepaliveTime                time.Duration
	KeepaliveTimeout             time.Duration
	KeepalivePermitWithoutStream bool

	// 压缩选项
	UseCompressor        string
	DefaultServiceConfig string

	// 健康检查选项
	HealthConfig *GRPCHealthConfig

	// 自定义 Dial 选项
	DialOptions []grpc.DialOption
}

// DefaultGRPCClientConfig 返回默认的 gRPC 客户端配置
func DefaultGRPCClientConfig() *GRPCClientConfig {
	return &GRPCClientConfig{
		Timeout:          20 * time.Second,
		KeepaliveTime:    30 * time.Second,
		KeepaliveTimeout: 10 * time.Second,
		HealthConfig: &GRPCHealthConfig{
			AllowedStates: map[connectivity.State]bool{
				connectivity.Ready: true,
				connectivity.Idle:  true,
			},
		},
	}
}

// GRPCConnectionFactory 用于创建 gRPC 客户端连接
type GRPCConnectionFactory struct {
	config *GRPCClientConfig
}

// NewGRPCConnectionFactory 创建一个新的 gRPC 连接工厂
func NewGRPCConnectionFactory(config *GRPCClientConfig) *GRPCConnectionFactory {
	if config == nil {
		config = DefaultGRPCClientConfig()
	}

	return &GRPCConnectionFactory{
		config: config,
	}
}

// Create 实现 ConnectionFactory 接口，创建一个新的 gRPC 客户端连接
func (f *GRPCConnectionFactory) Create(ctx context.Context) (pool.Connection, error) {
	dialOpts := make([]grpc.DialOption, 0)

	// 处理凭证
	if f.config.Secure {
		if f.config.TransportCredentials != nil {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(f.config.TransportCredentials))
		} else {
			// 这里可以添加默认的 TLS 配置
			// 示例: 创建默认的 TLS 凭证
			// creds := credentials.NewTLS(&tls.Config{
			//     InsecureSkipVerify: f.config.InsecureSkipVerify,
			//     ServerName:         f.config.ServerNameOverride,
			// })
			// dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		}
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 添加 PerRPC 凭证
	if f.config.PerRPCCredentials != nil {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(f.config.PerRPCCredentials))
	}

	// 处理拦截器
	for _, interceptor := range f.config.UnaryInterceptors {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(interceptor))
	}

	for _, interceptor := range f.config.StreamInterceptors {
		dialOpts = append(dialOpts, grpc.WithStreamInterceptor(interceptor))
	}

	// 处理连接选项
	if f.config.Block {
		dialOpts = append(dialOpts, grpc.WithBlock())
	}

	if f.config.DisableRetry {
		dialOpts = append(dialOpts, grpc.WithDisableRetry())
	}

	if f.config.DisableServiceConfig {
		dialOpts = append(dialOpts, grpc.WithDisableServiceConfig())
	}

	if f.config.MaxSendMsgSize > 0 {
		dialOpts = append(dialOpts, grpc.WithMaxMsgSize(f.config.MaxSendMsgSize))
	}

	if f.config.MaxRecvMsgSize > 0 {
		dialOpts = append(dialOpts, grpc.WithMaxMsgSize(f.config.MaxRecvMsgSize))
	}

	if f.config.UserAgent != "" {
		dialOpts = append(dialOpts, grpc.WithUserAgent(f.config.UserAgent))
	}

	// 处理保活选项
	if f.config.KeepaliveTime > 0 || f.config.KeepaliveTimeout > 0 {
		kaParams := keepalive.ClientParameters{
			Time:                f.config.KeepaliveTime,
			Timeout:             f.config.KeepaliveTimeout,
			PermitWithoutStream: f.config.KeepalivePermitWithoutStream,
		}
		dialOpts = append(dialOpts, grpc.WithKeepaliveParams(kaParams))
	}

	// 处理压缩选项
	if f.config.UseCompressor != "" {
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.UseCompressor(f.config.UseCompressor)))
	}

	if f.config.DefaultServiceConfig != "" {
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(f.config.DefaultServiceConfig))
	}

	// 添加自定义 Dial 选项
	dialOpts = append(dialOpts, f.config.DialOptions...)

	// 设置上下文超时
	var dialCtx context.Context
	var cancel context.CancelFunc
	if f.config.Timeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, f.config.Timeout)
		defer cancel()
	} else {
		dialCtx = ctx
	}

	// 建立连接
	grpcConn, err := grpc.DialContext(dialCtx, f.config.Target, dialOpts...)
	if err != nil {
		return nil, err
	}

	// 如果设置了阻塞模式，在此等待连接就绪
	if f.config.Block {
		state := grpcConn.GetState()
		if state != connectivity.Ready {
			if !grpcConn.WaitForStateChange(ctx, state) {
				// 上下文取消或超时
				grpcConn.Close()
				return nil, ctx.Err()
			}
		}
	}

	conn := &GRPCConnection{
		conn:      grpcConn,
		target:    f.config.Target,
		lastUsed:  time.Now(),
		healthCfg: f.config.HealthConfig,
	}

	return conn, nil
}

// GRPCClientPool 是 gRPC 连接池的助手
type GRPCClientPool struct {
	pool pool.Pool
}

// NewGRPCClientPool 创建一个新的 gRPC 客户端连接池
func NewGRPCClientPool(p pool.Pool) *GRPCClientPool {
	return &GRPCClientPool{
		pool: p,
	}
}

// GetConn 从池中获取 gRPC 连接
func (p *GRPCClientPool) GetConn(ctx context.Context) (*grpc.ClientConn, error) {
	conn, err := p.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	grpcConn, ok := conn.Raw().(*grpc.ClientConn)
	if !ok {
		conn.Close()
		return nil, p.pool.Put(conn, err)
	}

	return grpcConn, nil
}

// WithConnection 使用连接池中的连接执行操作
func (p *GRPCClientPool) WithConnection(ctx context.Context, fn func(*grpc.ClientConn) error) error {
	conn, err := p.pool.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	grpcConn, ok := conn.Raw().(*grpc.ClientConn)
	if !ok {
		return p.pool.Put(conn, err)
	}

	err = fn(grpcConn)
	return p.pool.Put(conn, err)
}

// Put 将连接放回池中
func (p *GRPCClientPool) Put(conn *grpc.ClientConn, err error) error {
	// 寻找原始 pool.Connection
	// 这需要池内部保持跟踪，这里是简化处理
	// 在实际使用中，应通过GRPCConnection引用放回池中
	return nil
}

// WithClient 使用动态创建的客户端执行操作
func (p *GRPCClientPool) WithClient(ctx context.Context, clientFactory func(*grpc.ClientConn) interface{}, fn func(client interface{}) error) error {
	return p.WithConnection(ctx, func(conn *grpc.ClientConn) error {
		client := clientFactory(conn)
		return fn(client)
	})
}

package adapters

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/fyerfyer/fyer-kit/pool"
)

// HTTPConnection 实现 Connection 接口，包装 http.Client
type HTTPConnection struct {
	client         *http.Client
	transport      *http.Transport
	lastUsed       time.Time
	keepAliveTimer *time.Timer
	mutex          sync.RWMutex
	closed         bool
}

// HTTPClientConfig 定义 HTTP 客户端连接的配置
type HTTPClientConfig struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration
	DisableKeepAlives     bool
	DisableCompression    bool
	TLSConfig             *tls.Config
}

// DefaultHTTPClientConfig 返回默认的 HTTP 客户端配置
func DefaultHTTPClientConfig() *HTTPClientConfig {
	return &HTTPClientConfig{
		Timeout:               30 * time.Second,
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
	}
}

// Close 实现 Connection 接口的关闭方法
func (c *HTTPConnection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	if c.keepAliveTimer != nil {
		c.keepAliveTimer.Stop()
		c.keepAliveTimer = nil
	}

	// 关闭 Transport 中的空闲连接
	c.transport.CloseIdleConnections()
	c.closed = true
	return nil
}

// Raw 返回底层的 http.Client
func (c *HTTPConnection) Raw() interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.client
}

// IsAlive 检查连接是否仍然可用
func (c *HTTPConnection) IsAlive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return !c.closed
}

// ResetState 为重用做准备
func (c *HTTPConnection) ResetState() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastUsed = time.Now()

	// 重置计时器
	if c.keepAliveTimer != nil {
		c.keepAliveTimer.Stop()
		c.keepAliveTimer = nil
	}

	return nil
}

// HTTPConnectionFactory 用于创建 HTTP 客户端连接
type HTTPConnectionFactory struct {
	config *HTTPClientConfig
}

// NewHTTPConnectionFactory 创建一个新的 HTTP 连接工厂
func NewHTTPConnectionFactory(config *HTTPClientConfig) *HTTPConnectionFactory {
	if config == nil {
		config = DefaultHTTPClientConfig()
	}

	return &HTTPConnectionFactory{
		config: config,
	}
}

// Create 实现 ConnectionFactory 接口，创建一个新的 HTTP 客户端连接
func (f *HTTPConnectionFactory) Create(ctx context.Context) (pool.Connection, error) {
	// 创建自定义的 Transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   f.config.DialTimeout,
			KeepAlive: f.config.KeepAlive,
		}).DialContext,
		MaxIdleConns:          f.config.MaxIdleConns,
		MaxIdleConnsPerHost:   f.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       f.config.MaxConnsPerHost,
		IdleConnTimeout:       f.config.IdleConnTimeout,
		TLSHandshakeTimeout:   f.config.TLSHandshakeTimeout,
		ExpectContinueTimeout: f.config.ExpectContinueTimeout,
		ResponseHeaderTimeout: f.config.ResponseHeaderTimeout,
		DisableKeepAlives:     f.config.DisableKeepAlives,
		DisableCompression:    f.config.DisableCompression,
		TLSClientConfig:       f.config.TLSConfig,
	}

	// 创建 HTTP 客户端
	client := &http.Client{
		Transport: transport,
		Timeout:   f.config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	conn := &HTTPConnection{
		client:    client,
		transport: transport,
		lastUsed:  time.Now(),
	}

	return conn, nil
}

// WithPooledClient 为给定的 HTTP 请求使用来自池的 HTTP 客户端
func WithPooledClient(pool pool.Pool, req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	conn, err := pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	httpConn, ok := conn.Raw().(*http.Client)
	if !ok {
		conn.Close()
		return nil, pool.Put(conn, err)
	}

	// 使用客户端发送请求
	resp, err := httpConn.Do(req)

	// 如果请求失败，标记连接为无效
	if err != nil {
		conn.Close()
		return nil, pool.Put(conn, err)
	}

	// 返回连接到池
	errPut := pool.Put(conn, nil)
	return resp, errPut
}

// PooledHTTPClient 是使用连接池的 HTTP 客户端
type PooledHTTPClient struct {
	pool pool.Pool
}

// NewPooledHTTPClient 创建一个使用连接池的 HTTP 客户端
func NewPooledHTTPClient(pool pool.Pool) *PooledHTTPClient {
	return &PooledHTTPClient{
		pool: pool,
	}
}

// Do 执行 HTTP 请求
func (c *PooledHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return WithPooledClient(c.pool, req)
}

// Get 执行 GET 请求
func (c *PooledHTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post 执行 POST 请求
func (c *PooledHTTPClient) Post(url, contentType string, body interface{}) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

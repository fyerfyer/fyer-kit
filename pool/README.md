# fyer-kit 连接池组件

一个功能丰富的Go语言连接池实现，提供高效的连接管理、多种连接适配器和完整的事件通知系统。

## 功能特点

- **通用连接池**：支持管理任何类型的连接资源
- **多种适配器**：内置SQL、Redis、HTTP、gRPC等适配器实现
- **连接限流**：内置多种限流算法（令牌桶、滑动窗口等）
- **健康检查**：支持定期检查连接活性，自动剔除无效连接
- **自动重连**：连接断开时自动重试
- **完整统计**：详细的连接池操作统计信息
- **事件通知**：提供丰富的事件系统，可监听连接状态变化
- **优雅关闭**：支持平滑关闭连接池
- **连接生命周期管理**：控制连接最大空闲时间和生命周期

## 架构设计

### 核心组件

```
pool/
├── interface.go     # 连接池接口定义
├── pool.go          # 连接池核心实现
├── options.go       # 连接池配置选项
├── connlimit/       # 连接限流组件
│   └── limiter.go   # 多种限流算法实现
└── adapters/        # 连接适配器
    ├── sql.go       # SQL数据库连接适配器
    ├── redis.go     # Redis连接适配器
    ├── http.go      # HTTP客户端连接适配器
    └── grpc.go      # gRPC客户端连接适配器
```

### 连接池工作流程

```
┌─────────────────────────┐      ┌─────────────────────────┐
│        Get              │      │         Put             │
└─────────────┬───────────┘      └─────────────┬───────────┘
              │                                │
              ▼                                ▼
      ┌───────────────┐                ┌───────────────┐
      │ 检查连接池状态│                │ 检查连接状态  │
      └───────┬───────┘                └───────┬───────┘
              │                                │
              ▼                                ▼
┌─────────────────────────┐      ┌─────────────────────────┐
│尝试从空闲池获取连接     │      │连接出错或不健康?        │
├─────────┬───────────────┤      ├─────────┬───────────────┤
│  成功   │     失败      │      │   是    │      否       │
▼         │               │      ▼         │               │
┌─────────▼───────────┐   │      ┌─────────▼───────────┐   │
│检查连接是否过期     │   │      │关闭并销毁连接       │   │
│或不健康            │   │      │                     │   │
└─────────┬───────────┘   │      └─────────────────────┘   │
          │               │                                │
          │               │                                │
          ▼               │                                ▼
┌─────────────────────┐   │      ┌───────────────────────────┐
│有效连接?            │   │      │重置连接状态               │
├─────────┬───────────┤   │      │更新最近使用时间           │
│   是    │    否     │   │      │                           │
▼         │           │   │      └─────────┬─────────────────┘
┌─────────▼───────────┐ │ │                │
│更新统计并返回连接   │ │ │                │
└───────────────────┘   │ │                │
                        │ │                ▼
                        │ │      ┌───────────────────────────┐
                        │ │      │添加到空闲连接池           │
                        │ │      │                           │
                        │ │      └───────────────────────────┘
                        │ │
                        ▼ ▼
      ┌───────────────────────────────────┐
      │检查是否达到最大连接数            │
      ├───────────┬───────────────────────┤
      │    是     │         否            │
      ▼           │                       │
┌─────────────────┐│                      │
│等待空闲连接     ││                      │
│或超时/取消      ││                      │
└─────────┬───────┘│                      │
          │        │                      │
          └────────┤                      │
                   ▼                      ▼
      ┌───────────────────────┐    ┌────────────────────┐
      │ 检查是否等待超时      │    │ 创建新连接         │
      │ 或达到重试次数        │    │                    │
      └─────────┬─────────────┘    └────────┬───────────┘
                │                           │
                ▼                           ▼
      ┌───────────────────────┐    ┌────────────────────┐
      │ 记录统计并返回错误    │    │ 更新统计并返回连接 │
      └───────────────────────┘    └────────────────────┘
```

### 连接池状态流转

```
┌─────────────┐
│ 连接创建    │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 状态:InUse  │◄────────┐
└──────┬──────┘         │
       │                │
       ▼                │
┌─────────────┐         │
│ Put回池     │         │ Get获取
└──────┬──────┘         │
       │                │
       ▼                │
┌─────────────┐         │
│ 状态:Idle   │─────────┘
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 连接关闭    │
└─────────────┘
```

| 特性 | 连接池 | 连接限制器 |
|------|--------|-----------|
| 核心并发机制 | 读写互斥锁+通道 | 令牌桶/滑动窗口算法 |
| 连接获取行为 | 无可用连接时等待或创建 | 超过限制时拒绝或等待 |
| 连接放回行为 | 放回池中或关闭连接 | - |
| 扩容机制 | 维持最小空闲连接数，最大总连接数 | 动态调整速率和并发数 |
| 性能特点 | 连接复用，减少创建开销 | 控制资源使用，防止过载 |
| 适用场景 | 数据库、缓存、HTTP、RPC等客户端 | 限制连接建立速率，防止过载 |
| 内部数据结构 | 通道+读写锁保护的集合 | 时间窗口或令牌桶 |

## 使用指南

### 基本用法

创建一个简单的连接池：

```go
// 创建连接工厂
factory := &customConnectionFactory{}

// 创建连接池
pool := pool.NewPool(factory,
    pool.WithMaxActive(10),
    pool.WithMaxIdle(5),
    pool.WithIdleCheckFrequency(30 * time.Second),
)

// 获取连接
ctx := context.Background()
conn, err := pool.Get(ctx)
if err != nil {
    // 处理错误
}

// 使用连接
// ...

// 归还连接
err = pool.Put(conn, nil)
if err != nil {
    // 处理错误
}

// 关闭连接池
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
pool.Shutdown(ctx)
```

### SQL 数据库适配器

```go
// 创建 SQL 数据库配置
config := adapters.DefaultSQLConfig("mysql", "user:password@tcp(localhost:3306)/dbname")
config.MaxOpenConns = 10
config.MaxIdleConns = 5
config.ConnMaxLifetime = 30 * time.Minute

// 创建 SQL 连接工厂
factory, err := adapters.NewSQLConnectionFactory(config)
if err != nil {
    // 处理错误
}

// 创建连接池
dbPool := pool.NewPool(factory,
    pool.WithTestOnBorrow(true),
    pool.WithMaxIdleTime(5 * time.Minute),
)

// 创建 SQL 助手
sqlHelper := adapters.NewSQLPoolHelper(dbPool)

// 执行查询
err = sqlHelper.Query(ctx, "SELECT * FROM users WHERE id = ?", []interface{}{1}, func(rows *sql.Rows) error {
    // 处理结果集
    for rows.Next() {
        var id int
        var name string
        if err := rows.Scan(&id, &name); err != nil {
            return err
        }
        fmt.Printf("User: %d, %s\n", id, name)
    }
    return nil
})

// 使用事务
err = sqlHelper.WithTransaction(ctx, nil, func(tx *sql.Tx) error {
    _, err := tx.Exec("INSERT INTO users(name) VALUES(?)", "张三")
    if err != nil {
        return err
    }
    return nil
})
```

### Redis 适配器

```go
// 创建 Redis 配置
config := adapters.DefaultRedisConfig()
config.Addr = "localhost:6379"
config.Password = "password"
config.DB = 0

// 创建 Redis 连接工厂
factory := adapters.NewRedisConnectionFactory(config)

// 创建连接池
redisPool := pool.NewPool(factory,
    pool.WithMaxActive(20),
    pool.WithMaxIdle(5),
)

// 创建 Redis 助手
redisHelper := adapters.NewRedisPoolHelper(redisPool)

// 执行Redis命令
result, err := redisHelper.Execute(ctx, func(client *redis.Client) (interface{}, error) {
    return client.Get(ctx, "key").Result()
})
if err != nil && err != redis.Nil {
    // 处理错误
}

// 使用命令执行器
cmd, err := redisHelper.ExecuteCmd(ctx, func(client *redis.Client) *redis.Cmd {
    return client.Set(ctx, "key", "value", 1*time.Hour)
})
if err != nil {
    // 处理错误
}
fmt.Println(cmd.String())
```

### HTTP 客户端适配器

```go
// 创建 HTTP 客户端配置
config := adapters.DefaultHTTPClientConfig()
config.Timeout = 10 * time.Second
config.MaxIdleConns = 20

// 创建 HTTP 连接工厂
factory := adapters.NewHTTPConnectionFactory(config)

// 创建连接池
httpPool := pool.NewPool(factory,
    pool.WithMaxActive(50),
    pool.WithMaxIdle(10),
)

// 创建带池化管理的 HTTP 客户端
client := adapters.NewPooledHTTPClient(httpPool)

// 发起 HTTP 请求
resp, err := client.Get("https://api.example.com/data")
if err != nil {
    // 处理错误
}
defer resp.Body.Close()

// 读取响应
data, err := ioutil.ReadAll(resp.Body)
if err != nil {
    // 处理错误
}
fmt.Println(string(data))
```

### gRPC 客户端适配器

```go
// 创建 gRPC 客户端配置
config := adapters.DefaultGRPCClientConfig()
config.Target = "localhost:50051"
config.Secure = false

// 创建 gRPC 连接工厂
factory := adapters.NewGRPCConnectionFactory(config)

// 创建连接池
grpcPool := pool.NewPool(factory,
    pool.WithMaxActive(30),
    pool.WithMaxIdle(5),
    pool.WithDialTimeout(5 * time.Second),
)

// 创建 gRPC 客户端池
clientPool := adapters.NewGRPCClientPool(grpcPool)

// 使用 gRPC 连接执行 RPC 调用
err := clientPool.WithConnection(ctx, func(conn *grpc.ClientConn) error {
    client := pb.NewYourServiceClient(conn)
    resp, err := client.YourMethod(ctx, &pb.YourRequest{})
    if err != nil {
        return err
    }
    fmt.Printf("Response: %v\n", resp)
    return nil
})
```

### 连接限流器使用示例

```go
// 创建令牌桶限流器
limiter := connlimit.NewTokenBucketLimiter(100, 20,
    connlimit.WithMaxWaitTime(500 * time.Millisecond))

// 检查是否允许连接
if !limiter.Allow() {
    // 连接被限流
    fmt.Println("connection rejected by limiter")
    return
}

// 等待直到可以建立连接
err := limiter.Wait(ctx)
if err != nil {
    // 处理等待超时或上下文取消
    if errors.Is(err, connlimit.ErrWaitTimeout) {
        fmt.Println("wait timeout")
    }
    return
}

// 创建滑动窗口限流器
windowLimiter := connlimit.NewWindowLimiter(time.Second, 50,
    connlimit.WithWindowMaxWaitTime(300 * time.Millisecond))

// 自适应限流器（结合两种限流策略）
adaptiveLimiter := connlimit.NewAdaptiveLimiter(
    limiter, windowLimiter, connlimit.AllRequired)
```

## 高级特性

### 事件监听

```go
// 定义事件监听器
listener := &CustomEventListener{}

// 创建带事件监听的连接池
pool := pool.NewPool(factory,
    pool.WithEventListener(listener),
)

// 自定义事件监听器实现
type CustomEventListener struct{}

func (l *CustomEventListener) OnEvent(event pool.Event, conn pool.Connection) {
    switch event {
    case pool.EventNew:
        fmt.Println("create new connection")
    case pool.EventGet:
        fmt.Println("get connection")
    case pool.EventPut:
        fmt.Println("put connection")
    case pool.EventClose:
        fmt.Println("close connection")
    }
}
```

### 连接健康检查

```go
// 自定义健康检查
healthCheck := func(conn pool.Connection) bool {
    // 实现自定义健康检查逻辑
    sqlConn, ok := conn.Raw().(*sql.DB)
    if !ok {
        return false
    }
    return sqlConn.PingContext(context.Background()) == nil
}

// 使用健康检查创建连接池
pool := pool.NewPool(factory,
    pool.WithHealthCheck(healthCheck),
    pool.WithTestOnBorrow(true),
    pool.WithTestOnReturn(true),
)
```

### 统计信息

```go
// 获取连接池统计信息
stats := pool.Stats()

fmt.Printf("active connection: %d\n", stats.Active)
fmt.Printf("idle connection: %d\n", stats.Idle)
fmt.Printf("totoal connection: %d\n", stats.Total)
fmt.Printf("await connection: %d\n", stats.Waiters)
fmt.Printf("timeout times: %d\n", stats.Timeouts)
fmt.Printf("error times: %d\n", stats.Errors)
fmt.Printf("get connections times: %d\n", stats.Acquired)
fmt.Printf("put connections times: %d\n", stats.Released)
fmt.Printf("connection pool created at: %s\n", stats.CreatedAt)
```

### 自定义连接和工厂

```go
// 实现自定义连接
type MyConnection struct {
    client    *MyClient
    lastUsed  time.Time
    closed    bool
    mu        sync.RWMutex
}

// 实现 Connection 接口
func (c *MyConnection) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.closed {
        return nil
    }
    c.closed = true
    return c.client.Close()
}

func (c *MyConnection) Raw() interface{} {
    return c.client
}

func (c *MyConnection) IsAlive() bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return !c.closed && c.client.IsConnected()
}

func (c *MyConnection) ResetState() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.lastUsed = time.Now()
    return nil
}

// 实现连接工厂
type MyConnectionFactory struct {
    addr     string
    username string
    password string
}

// 实现 ConnectionFactory 接口
func (f *MyConnectionFactory) Create(ctx context.Context) (pool.Connection, error) {
    client, err := NewMyClient(f.addr, f.username, f.password)
    if err != nil {
        return nil, err
    }
    
    return &MyConnection{
        client:   client,
        lastUsed: time.Now(),
    }, nil
}
```

### 优雅关闭

```go
// 创建带超时的上下文
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// 优雅关闭连接池
err := pool.Shutdown(ctx)
if err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        fmt.Println("failed to shutdown pool: timeout")
    } else {
        fmt.Printf("failed to shutdown pool: %v\n", err)
    }
}
```

## 配置选项

连接池提供以下配置选项：

- `WithInitialSize(size)`: 设置初始连接数
- `WithMaxIdle(maxIdle)`: 设置最大空闲连接数
- `WithMaxActive(maxActive)`: 设置最大活动连接数
- `WithMaxIdleTime(duration)`: 设置连接最大空闲时间
- `WithMaxLifetime(duration)`: 设置连接最大生命周期
- `WithIdleCheckFrequency(frequency)`: 设置空闲连接检查频率
- `WithWaitTimeout(timeout)`: 设置等待连接超时时间
- `WithMaxWaiters(maxWaiters)`: 设置最大等待者数量
- `WithMinEvictableIdle(minIdle)`: 设置触发驱逐检查的最小空闲连接数
- `WithOnCreate(callback)`: 设置创建连接时的回调函数
- `WithOnClose(callback)`: 设置关闭连接时的回调函数
- `WithHealthCheck(healthCheck)`: 设置连接健康检查函数
- `WithRetryBackoff(strategy)`: 设置重试退避策略
- `WithMaxRetries(maxRetries)`: 设置最大重试次数
- `WithDialTimeout(timeout)`: 设置连接超时时间
- `WithTestOnBorrow(test)`: 设置是否在借用连接时测试连接
- `WithTestOnReturn(test)`: 设置是否在归还连接时测试连接
- `WithEventListener(listener)`: 添加事件监听器
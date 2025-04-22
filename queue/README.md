# fyer-kit 队列组件

一个功能丰富的Go语言队列实现，提供阻塞和非阻塞操作，支持有界和无界队列，具有完整的事件通知和统计信息。

## 功能特点

- **多种队列类型**：支持阻塞队列和非阻塞队列
- **灵活容量**：支持有界队列和无界队列（动态扩容）
- **超时控制**：可为入队和出队操作设置超时时间
- **事件通知**：提供丰富的事件系统，可监听队列状态变化
- **完整统计**：详细的队列操作统计信息
- **泛型支持**：基于Go泛型实现，支持任意类型
- **服务层抽象**：提供高级队列服务接口
- **命令行工具**：便捷的队列管理CLI工具

## 架构设计

### 核心组件

```
queue/
├── queue.go         # 队列接口定义
├── blocking_queue.go # 阻塞队列实现
├── nonblocking_queue.go # 非阻塞队列实现
├── options.go       # 队列配置选项
├── stats.go         # 队列统计信息
├── event.go         # 事件系统
└── errors.go        # 错误定义

internal/queueservice/
├── service.go       # 队列服务接口
├── inmemory.go      # 内存队列服务实现
└── serilizer.go     # 队列序列化工具

cmd/qcli/            # 命令行工具
```

## 队列架构与工作流程

```
┌─────────────────────────┐      ┌─────────────────────────┐
│        Enqueue          │      │        Dequeue          │
└─────────────┬───────────┘      └─────────────┬───────────┘
              │                                │
              ▼                                ▼
      ┌───────────────┐                ┌───────────────┐
      │ 获取互斥锁    │                │ 获取互斥锁    │
      └───────┬───────┘                └───────┬───────┘
              │                                │
              ▼                                ▼
      ┌───────────────┐                ┌───────────────┐
      │ 检查队列状态  │                │ 检查队列状态  │
      └───────┬───────┘                └───────┬───────┘
              │                                │
              ▼                                ▼
┌─────────────────────────┐      ┌─────────────────────────┐
│队列已满?                │      │队列为空?                │
├─────────┬───────────────┤      ├─────────┬───────────────┤
│  是     │      否       │      │  是     │      否       │
▼         │               │      ▼         │               │
┌─────────▼───────────┐   │      ┌─────────▼───────────┐   │
│等待不满条件变量     │   │      │等待不空条件变量     │   │
│(挂起并释放互斥锁)   │   │      │(挂起并释放互斥锁)   │   │
└─────────┬───────────┘   │      └─────────┬───────────┘   │
          │               │                │               │
          ▼               │                ▼               │
┌─────────────────────┐   │      ┌─────────────────────┐   │
│等待期间检查:        │   │      │等待期间检查:        │   │
│- 超时是否发生       │   │      │- 超时是否发生       │   │
│- 上下文是否取消     │   │      │- 上下文是否取消     │   │
│- 队列是否已关闭     │   │      │- 队列是否已关闭     │   │
└─────────┬───────────┘   │      └─────────┬───────────┘   │
          │               │                │               │
          └───────┐       │                └───────┐       │
                  ▼       ▼                        ▼       ▼
          ┌───────────────────┐              ┌───────────────────┐
          │ 将元素添加到队列  │              │ 从队列取出元素    │
          └─────────┬─────────┘              └─────────┬─────────┘
                    │                                  │
                    ▼                                  ▼
          ┌───────────────────┐              ┌───────────────────┐
          │ 更新队列状态      │              │ 更新队列状态      │
          └─────────┬─────────┘              └─────────┬─────────┘
                    │                                  │
                    ▼                                  ▼
          ┌───────────────────┐              ┌───────────────────┐
          │ 如果队列从空变为非│              │ 如果队列从满变为非│
          │ 空，唤醒消费者    │              │ 满，唤醒生产者    │
          └─────────┬─────────┘              └─────────┬─────────┘
                    │                                  │
                    ▼                                  ▼
          ┌───────────────────┐              ┌───────────────────┐
          │ 更新统计与发送事件│              │ 更新统计与发送事件│
          └─────────┬─────────┘              └─────────┬─────────┘
                    │                                  │
                    ▼                                  ▼
          ┌───────────────────┐              ┌───────────────────┐
          │ 释放互斥锁        │              │ 释放互斥锁        │
          └───────────────────┘              └───────────────────┘
```

```
┌─────────────────────────┐      ┌─────────────────────────┐
│   Enqueue/TryEnqueue    │      │   Dequeue/TryDequeue    │
└─────────────┬───────────┘      └─────────────┬───────────┘
              │                                │
              ▼                                ▼
      ┌───────────────┐                ┌───────────────┐
      │ 检查上下文与  │                │ 检查上下文与  │
      │ 队列关闭状态  │                │ 队列为空状态  │
      └───────┬───────┘                └───────┬───────┘
              │                                │
              ▼                                ▼
┌─────────────────────────┐      ┌─────────────────────────┐
│队列已满且有界?          │      │队列为空?                │
├─────────┬───────────────┤      ├─────────┬───────────────┤
│  是     │      否       │      │  是     │      否       │
▼         │               │      ▼         │               │
┌─────────▼───────────┐   │      ┌─────────▼───────────┐   │
│立即返回ErrQueueFull │   │      │立即返回ErrQueueEmpty│   │
└───────────────────┘     │      └───────────────────┘     │
                          │                                │
                          ▼                                ▼
          ┌───────────────────────┐      ┌───────────────────────┐
          │如果队列已满且无界:    │      │CAS循环:               │
          │获取锁并扩容队列       │      │1. 获取head、size快照  │
          └─────────┬─────────────┘      │2. 尝试原子更新head   │
                    │                    │3. 如果失败则重试     │
                    ▼                    └─────────┬────────────┘
          ┌───────────────────────┐                │
          │CAS循环:               │                │
          │1. 获取tail、size快照  │                │
          │2. 尝试原子更新tail    │                │
          │3. 如果失败则重试      │                │
          └─────────┬─────────────┘                │
                    │                              │
                    ▼                              ▼
          ┌───────────────────────┐      ┌───────────────────────┐
          │更新统计信息          │      │更新统计信息          │
          │(使用互斥锁保护)      │      │(使用互斥锁保护)      │
          └─────────┬─────────────┘      └─────────┬────────────┘
                    │                              │
                    ▼                              ▼
          ┌───────────────────────┐      ┌───────────────────────┐
          │发送相关事件          │      │发送相关事件          │
          └───────────────────────┘      └───────────────────────┘
```

| 特性 | 阻塞队列 | 非阻塞队列 |
|------|---------|-----------|
| 核心并发机制 | 互斥锁+条件变量 | 原子操作+CAS |
| Enqueue行为 | 队列满时阻塞等待 | 队列满时立即返回错误 |
| Dequeue行为 | 队列空时阻塞等待 | 队列空时立即返回错误 |
| 扩容机制 | 互斥锁保护扩容 | 互斥锁保护扩容，原子操作更新头尾 |
| 性能特点 | 保证背压控制，可能有锁争用 | 高吞吐量，无阻塞延迟 |
| 适用场景 | 需要背压控制的生产者-消费者模型 | 高并发、允许操作失败的场景 |
| 内部数据结构 | 循环数组 | 循环数组 |
| 使用锁的场景 | 所有队列核心操作 | 仅用于扩容和统计信息更新 |

## 使用指南

### 基本用法

创建一个简单的队列：

```go
// 创建一个容量为10的阻塞队列
q := queue.NewQueue[string](queue.WithCapacity(10))

// 入队元素
ctx := context.Background()
err := q.Enqueue(ctx, "hello")
if err != nil {
    // 处理错误
}

// 出队元素
item, err := q.Dequeue(ctx)
if err != nil {
    // 处理错误
}
fmt.Println(item) // 输出：hello
```

### 非阻塞操作

```go
// 非阻塞入队
err := q.TryEnqueue("world")
if errors.Is(err, queue.ErrQueueFull) {
    fmt.Println("queue already full")
}

// 非阻塞出队
item, err := q.TryDequeue()
if errors.Is(err, queue.ErrQueueEmpty) {
    fmt.Println("queue already empty")
}
```

### 使用超时

```go
// 创建带超时的队列
timeoutQueue := queue.NewQueue[int](
    queue.WithCapacity(5),
    queue.WithEnqueueTimeout(100 * time.Millisecond),
    queue.WithDequeueTimeout(200 * time.Millisecond),
)

// 入队可能超时
err := timeoutQueue.Enqueue(context.Background(), 42)
if errors.Is(err, queue.ErrOperationTimeout) {
    fmt.Println("queue enqueue timeout")
}
```

### 事件监听

```go
// 定义事件监听器
listener := func(evt queue.Event) {
    switch evt.Type {
    case queue.EventEnqueue:
        fmt.Printf("elem enqueued，current size：%d\n", evt.Size)
    case queue.EventDequeue:
        fmt.Printf("elem dequeued，current size：%d\n", evt.Size)
    case queue.EventFull:
        fmt.Println("queue already full")
    case queue.EventEmpty:
        fmt.Println("queue already empty")
    }
}

// 创建带监听器的队列
q := queue.NewQueue[string](
    queue.WithCapacity(3),
    queue.WithEventListener(listener),
)
```

### 使用队列服务

队列服务提供了更高级的队列管理能力：

```go
// 创建队列服务
service := queueservice.NewInMemoryService()

// 创建队列
err := service.CreateQueue("my-queue", queueservice.QueueOptions{
    Type:     queueservice.BlockingQueue,
    Capacity: 100,
})

// 获取队列
q, err := service.GetQueue("my-queue")
if err != nil {
    // 处理错误
}

// 入队
err = service.EnqueueItem("my-queue", "hello")

// 出队
item, err := service.DequeueItem("my-queue")

// 获取队列状态
stats, err := service.QueueStats("my-queue")
fmt.Printf("queue size: %d/%d\n", stats.Size, stats.Capacity)

// 列出所有队列
queues := service.ListQueues()
for _, qInfo := range queues {
    fmt.Printf("queue: %s, type: %s, size: %d\n", 
        qInfo.Name, qInfo.Type, qInfo.Stats.Size)
}

// 删除队列
err = service.DeleteQueue("my-queue")

// 关闭服务
service.Close()
```

## 高级特性

### 无界队列

无界队列会在需要时自动扩容：

```go
// 创建无界队列（不指定容量）
q := queue.NewQueue[int]()

// 可以添加任意数量的元素，队列会自动扩容
for i := 0; i < 10000; i++ {
    q.TryEnqueue(i)
}
```

### 统计信息

```go
// 获取队列统计信息
stats := q.Stats()
fmt.Printf("queue size: %d\n", stats.Size)
fmt.Printf("queue capacity: %d\n", stats.Capacity)
fmt.Printf("enqueue: %d\n", stats.Enqueued)
fmt.Printf("dequeue: %d\n", stats.Dequeued)
fmt.Printf("enqueue blocking times: %d\n", stats.EnqueueBlocks)
fmt.Printf("dequeue blocking times: %d\n", stats.DequeueBlocks)
fmt.Printf("enqueue timeout times: %d\n", stats.EnqueueTimeouts)
fmt.Printf("dequeue timeout times: %d\n", stats.DequeueTimeouts)
fmt.Printf("declined enqueue operations: %d\n", stats.Rejected)
fmt.Printf("queue utilization: %.2f%%\n", stats.Utilization()*100)
```

### 队列关闭

```go
// 关闭队列
err := q.Close()
if err != nil {
    // 处理错误
}

// 关闭后，入队会失败但仍可出队已有元素
err = q.TryEnqueue("new-item") // 返回 ErrQueueClosed
item, err = q.TryDequeue() // 可以获取队列中的元素
```

## 配置选项

队列提供以下配置选项：

- `WithCapacity(n)`: 设置队列容量，0表示无界队列
- `WithEnqueueTimeout(d)`: 设置入队操作的超时时间
- `WithDequeueTimeout(d)`: 设置出队操作的超时时间
- `WithEventListener(fn)`: 添加事件监听器
- `WithEventListeners(fns)`: 批量添加事件监听器

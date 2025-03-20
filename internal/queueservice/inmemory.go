package queueservice

import (
	"sync"
	"time"

	"github.com/fyerfyer/fyer-kit/queue"
)

// InMemoryService 实现了Service接口的内存存储版本
type InMemoryService struct {
	// 队列名称到队列实例的映射
	queues map[string]queueEntry
	// 保护映射的互斥锁
	mu sync.RWMutex
}

// queueEntry 包含队列及其元数据
type queueEntry struct {
	// 队列实例
	q queue.Queue[string]
	// 队列类型
	qType QueueType
	// 创建时间
	createdAt time.Time
}

// NewInMemoryService 创建一个新的内存队列服务
func NewInMemoryService() *InMemoryService {
	return &InMemoryService{
		queues: make(map[string]queueEntry),
	}
}

// CreateQueue 创建一个新队列
func (s *InMemoryService) CreateQueue(name string, opts QueueOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查队列是否已存在
	if _, exists := s.queues[name]; exists {
		return ErrQueueExists
	}

	// 创建队列选项
	queueOpts := []queue.Option{
		queue.WithCapacity(opts.Capacity),
	}

	if opts.EnqueueTimeout > 0 {
		queueOpts = append(queueOpts,
			queue.WithEnqueueTimeout(time.Duration(opts.EnqueueTimeout)*time.Millisecond))
	}

	if opts.DequeueTimeout > 0 {
		queueOpts = append(queueOpts,
			queue.WithDequeueTimeout(time.Duration(opts.DequeueTimeout)*time.Millisecond))
	}

	// 创建相应类型的队列
	var q queue.Queue[string]
	if opts.Type == BlockingQueue {
		q = queue.NewBlockingQueue[string](queueOpts...)
	} else {
		q = queue.NewNonBlockingQueue[string](queueOpts...)
	}

	// 存储队列
	s.queues[name] = queueEntry{
		q:         q,
		qType:     opts.Type,
		createdAt: time.Now(),
	}

	return nil
}

// GetQueue 获取指定名称的队列
func (s *InMemoryService) GetQueue(name string) (queue.Queue[string], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.queues[name]
	if !exists {
		return nil, ErrQueueNotFound
	}

	return entry.q, nil
}

// ListQueues 列出所有队列
func (s *InMemoryService) ListQueues() []QueueInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]QueueInfo, 0, len(s.queues))
	for name, entry := range s.queues {
		stats := entry.q.Stats()
		result = append(result, QueueInfo{
			Name:  name,
			Type:  entry.qType,
			Stats: stats,
		})
	}

	return result
}

// EnqueueItem 向指定队列添加项目
func (s *InMemoryService) EnqueueItem(queueName string, item string) error {
	q, err := s.GetQueue(queueName)
	if err != nil {
		return err
	}

	// 对于CLI操作，使用非阻塞的TryEnqueue更友好
	return q.TryEnqueue(item)
}

// DequeueItem 从指定队列获取项目
func (s *InMemoryService) DequeueItem(queueName string) (string, error) {
	q, err := s.GetQueue(queueName)
	if err != nil {
		return "", err
	}

	// 对于CLI操作，使用非阻塞的TryDequeue更友好
	return q.TryDequeue()
}

// QueueStats 获取队列统计信息
func (s *InMemoryService) QueueStats(queueName string) (queue.Stats, error) {
	q, err := s.GetQueue(queueName)
	if err != nil {
		return queue.Stats{}, err
	}

	return q.Stats(), nil
}

// DeleteQueue 删除队列
func (s *InMemoryService) DeleteQueue(queueName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.queues[queueName]
	if !exists {
		return ErrQueueNotFound
	}

	// 关闭队列后删除
	_ = entry.q.Close()
	delete(s.queues, queueName)

	return nil
}

// Close 关闭所有队列
func (s *InMemoryService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range s.queues {
		_ = entry.q.Close()
	}

	s.queues = make(map[string]queueEntry)
	return nil
}

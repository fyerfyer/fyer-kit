package queueservice

import (
	"errors"

	"github.com/fyerfyer/fyer-kit/queue"
)

var (
	// ErrQueueNotFound 表示请求的队列不存在
	ErrQueueNotFound = errors.New("queue not found")

	// ErrQueueExists 表示队列已存在
	ErrQueueExists = errors.New("queue already exists")
)

// QueueType 定义队列类型
type QueueType string

const (
	// BlockingQueue 阻塞队列类型
	BlockingQueue QueueType = "blocking"
	// NonBlockingQueue 非阻塞队列类型
	NonBlockingQueue QueueType = "nonblocking"
)

// QueueOptions 表示创建队列时的选项
type QueueOptions struct {
	// 队列类型
	Type QueueType
	// 队列容量
	Capacity int
	// 入队超时时间（毫秒）
	EnqueueTimeout int
	// 出队超时时间（毫秒）
	DequeueTimeout int
}

// QueueInfo 包含队列的基本信息
type QueueInfo struct {
	// 队列名称
	Name string
	// 队列类型
	Type QueueType
	// 队列状态
	Stats queue.Stats
}

// Service 定义队列服务接口
type Service interface {
	// CreateQueue 创建一个新队列
	CreateQueue(name string, opts QueueOptions) error

	// GetQueue 获取指定名称的队列
	GetQueue(name string) (queue.Queue[string], error)

	// ListQueues 列出所有队列
	ListQueues() []QueueInfo

	// EnqueueItem 向指定队列添加项目
	EnqueueItem(queueName string, item string) error

	// DequeueItem 从指定队列获取项目
	DequeueItem(queueName string) (string, error)

	// QueueStats 获取队列统计信息
	QueueStats(queueName string) (queue.Stats, error)

	// DeleteQueue 删除队列
	DeleteQueue(queueName string) error

	// Close 关闭所有队列
	Close() error
}

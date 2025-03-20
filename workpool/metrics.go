package workpool

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 包含工作池的运行时指标
type Metrics struct {
	// 任务相关指标
	TotalTasks     uint64        // 总提交任务数
	CompletedTasks uint64        // 已完成任务数
	FailedTasks    uint64        // 失败任务数
	CanceledTasks  uint64        // 取消任务数
	QueuedTasks    uint64        // 当前排队任务数
	AvgWaitTime    time.Duration // 平均等待时间
	AvgProcessTime time.Duration // 平均处理时间

	// 工作池状态
	ActiveWorkers int32 // 当前活跃工作协程数
	IdleWorkers   int32 // 当前空闲工作协程数
	TotalWorkers  int32 // 当前总工作协程数
	PeakWorkers   int32 // 峰值工作协程数

	// 内部统计数据
	totalWaitTime    int64
	totalProcessTime int64

	mu sync.RWMutex
}

// newMetrics 创建一个新的指标收集器
func newMetrics() *Metrics {
	return &Metrics{}
}

// Reset 重置所有指标
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.StoreUint64(&m.TotalTasks, 0)
	atomic.StoreUint64(&m.CompletedTasks, 0)
	atomic.StoreUint64(&m.FailedTasks, 0)
	atomic.StoreUint64(&m.CanceledTasks, 0)
	atomic.StoreUint64(&m.QueuedTasks, 0)

	atomic.StoreInt32(&m.ActiveWorkers, 0)
	atomic.StoreInt32(&m.IdleWorkers, 0)
	atomic.StoreInt32(&m.TotalWorkers, 0)
	atomic.StoreInt32(&m.PeakWorkers, 0)

	m.totalWaitTime = 0
	m.totalProcessTime = 0
	m.AvgWaitTime = 0
	m.AvgProcessTime = 0
}

// taskSubmitted 记录任务提交
func (m *Metrics) taskSubmitted() {
	atomic.AddUint64(&m.TotalTasks, 1)
	atomic.AddUint64(&m.QueuedTasks, 1)
}

// taskStarted 记录任务开始执行
func (m *Metrics) taskStarted(waitTime time.Duration) {
	atomic.AddUint64(&m.QueuedTasks, ^uint64(0)) // 减1

	m.mu.Lock()
	defer m.mu.Unlock()

	// 更新等待时间统计
	waitNanos := int64(waitTime)
	m.totalWaitTime += waitNanos

	// 更新平均等待时间
	completed := atomic.LoadUint64(&m.CompletedTasks)
	failed := atomic.LoadUint64(&m.FailedTasks)
	canceled := atomic.LoadUint64(&m.CanceledTasks)

	totalFinished := completed + failed + canceled
	if totalFinished > 0 {
		m.AvgWaitTime = time.Duration(m.totalWaitTime / int64(totalFinished+1))
	}
}

// taskCompleted 记录任务完成
func (m *Metrics) taskCompleted(processingTime time.Duration, success bool) {
	if success {
		atomic.AddUint64(&m.CompletedTasks, 1)
	} else {
		atomic.AddUint64(&m.FailedTasks, 1)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 更新处理时间统计
	processNanos := int64(processingTime)
	m.totalProcessTime += processNanos

	// 更新平均处理时间
	completed := atomic.LoadUint64(&m.CompletedTasks)
	failed := atomic.LoadUint64(&m.FailedTasks)

	totalProcessed := completed + failed
	if totalProcessed > 0 {
		m.AvgProcessTime = time.Duration(m.totalProcessTime / int64(totalProcessed))
	}
}

// taskCanceled 记录任务被取消
func (m *Metrics) taskCanceled() {
	atomic.AddUint64(&m.CanceledTasks, 1)
	atomic.AddUint64(&m.QueuedTasks, ^uint64(0)) // 减1
}

// workerStatusChanged 更新工作协程状态指标
func (m *Metrics) workerStatusChanged(active, idle, total int32) {
	atomic.StoreInt32(&m.ActiveWorkers, active)
	atomic.StoreInt32(&m.IdleWorkers, idle)
	atomic.StoreInt32(&m.TotalWorkers, total)

	// 更新峰值工作协程数
	for {
		current := atomic.LoadInt32(&m.PeakWorkers)
		if total <= current || atomic.CompareAndSwapInt32(&m.PeakWorkers, current, total) {
			break
		}
	}
}

// Snapshot 返回当前指标的快照
func (m *Metrics) Snapshot() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return Metrics{
		TotalTasks:     atomic.LoadUint64(&m.TotalTasks),
		CompletedTasks: atomic.LoadUint64(&m.CompletedTasks),
		FailedTasks:    atomic.LoadUint64(&m.FailedTasks),
		CanceledTasks:  atomic.LoadUint64(&m.CanceledTasks),
		QueuedTasks:    atomic.LoadUint64(&m.QueuedTasks),
		AvgWaitTime:    m.AvgWaitTime,
		AvgProcessTime: m.AvgProcessTime,
		ActiveWorkers:  atomic.LoadInt32(&m.ActiveWorkers),
		IdleWorkers:    atomic.LoadInt32(&m.IdleWorkers),
		TotalWorkers:   atomic.LoadInt32(&m.TotalWorkers),
		PeakWorkers:    atomic.LoadInt32(&m.PeakWorkers),
	}
}

// WorkerUtilization 计算工作协程的利用率 (0.0-1.0)
func (m *Metrics) WorkerUtilization() float64 {
	total := atomic.LoadInt32(&m.TotalWorkers)
	if total == 0 {
		return 0.0
	}

	active := atomic.LoadInt32(&m.ActiveWorkers)
	return float64(active) / float64(total)
}

// QueueUtilization 计算队列的利用率 (0.0-1.0)，需要传入队列容量
func (m *Metrics) QueueUtilization(capacity uint64) float64 {
	if capacity == 0 {
		return 0.0
	}

	queued := atomic.LoadUint64(&m.QueuedTasks)
	if queued > capacity {
		return 1.0
	}

	return float64(queued) / float64(capacity)
}

// TaskSuccessRate 计算任务成功率 (0.0-1.0)
func (m *Metrics) TaskSuccessRate() float64 {
	completed := atomic.LoadUint64(&m.CompletedTasks)
	failed := atomic.LoadUint64(&m.FailedTasks)

	total := completed + failed
	if total == 0 {
		return 1.0
	}

	return float64(completed) / float64(total)
}

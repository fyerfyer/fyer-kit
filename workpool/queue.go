package workpool

import (
	"container/heap"
	"sync"
)

// priorityQueue 实现优先级队列，用于管理具有不同优先级的任务
type priorityQueue struct {
	items []*taskHandle
	mu    sync.Mutex
}

// newPriorityQueue 创建一个新的优先级队列
func newPriorityQueue() *priorityQueue {
	pq := &priorityQueue{
		items: make([]*taskHandle, 0),
	}
	heap.Init(pq)
	return pq
}

// 实现 heap.Interface 接口所需的方法

// Len 返回队列长度
func (pq *priorityQueue) Len() int {
	return len(pq.items)
}

// Less 比较两个任务的优先级，高优先级的任务排在前面
func (pq *priorityQueue) Less(i, j int) bool {
	// 注意：这里使用 > 而不是 < 以便高优先级的任务排在前面
	return pq.items[i].config.priority > pq.items[j].config.priority
}

// Swap 交换两个任务的位置
func (pq *priorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push 向队列中添加一个任务
func (pq *priorityQueue) Push(x interface{}) {
	item := x.(*taskHandle)
	pq.items = append(pq.items, item)
}

// Pop 从队列中取出优先级最高的任务
func (pq *priorityQueue) Pop() interface{} {
	old := pq.items
	n := len(old)
	item := old[n-1]
	pq.items = old[0 : n-1]
	return item
}

// 以下是线程安全的外部方法

// Enqueue 添加任务到队列中
func (pq *priorityQueue) Enqueue(task *taskHandle) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	heap.Push(pq, task)
}

// Dequeue 从队列中取出优先级最高的任务，如果队列为空则返回nil
func (pq *priorityQueue) Dequeue() *taskHandle {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	return heap.Pop(pq).(*taskHandle)
}

// Size 返回队列中的任务数量
func (pq *priorityQueue) Size() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items)
}

// IsEmpty 检查队列是否为空
func (pq *priorityQueue) IsEmpty() bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items) == 0
}

// Peek 查看队列中优先级最高的任务但不移除它，如果队列为空则返回nil
func (pq *priorityQueue) Peek() *taskHandle {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	return pq.items[0]
}

// Clear 清空队列
func (pq *priorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.items = make([]*taskHandle, 0)
}

// Remove 从队列中移除特定ID的任务
// 如果找到并移除则返回true，否则返回false
func (pq *priorityQueue) Remove(id string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, task := range pq.items {
		if task.id == id {
			// 使用堆操作移除元素
			heap.Remove(pq, i)
			return true
		}
	}

	return false
}

package slice

import (
	"sync"
)

// Transaction 提供对 SafeSlice 的事务操作
type Transaction[T any] struct {
	original    *SafeSlice[T]
	workingCopy []T
	committed   bool
	mu          sync.Mutex
}

// Begin 开始一个新事务
func (s *SafeSlice[T]) Begin() *Transaction[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 创建工作副本
	workingCopy := make([]T, len(s.items))
	copy(workingCopy, s.items)

	return &Transaction[T]{
		original:    s,
		workingCopy: workingCopy,
		committed:   false,
	}
}

// Get 获取事务中指定索引的元素
func (tx *Transaction[T]) Get(index int) (T, bool) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	var zero T
	if tx.committed {
		return zero, false
	}

	if index < 0 || index >= len(tx.workingCopy) {
		return zero, false
	}

	return tx.workingCopy[index], true
}

// Set 在事务中设置指定索引的元素值
func (tx *Transaction[T]) Set(index int, value T) bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return false
	}

	if index < 0 || index >= len(tx.workingCopy) {
		return false
	}

	tx.workingCopy[index] = value
	return true
}

// Append 在事务中添加元素
func (tx *Transaction[T]) Append(items ...T) bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return false
	}

	tx.workingCopy = append(tx.workingCopy, items...)
	return true
}

// Delete 在事务中删除指定索引的元素
func (tx *Transaction[T]) Delete(index int) bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return false
	}

	if index < 0 || index >= len(tx.workingCopy) {
		return false
	}

	tx.workingCopy = append(tx.workingCopy[:index], tx.workingCopy[index+1:]...)
	return true
}

// Len 返回事务中切片的长度
func (tx *Transaction[T]) Len() int {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return 0
	}

	return len(tx.workingCopy)
}

// Commit 提交事务，将更改应用到原始切片
func (tx *Transaction[T]) Commit() bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return false
	}

	// 获取原始切片的锁，确保原子性
	tx.original.mu.Lock()
	defer tx.original.mu.Unlock()

	// 应用更改
	tx.original.items = make([]T, len(tx.workingCopy))
	copy(tx.original.items, tx.workingCopy)

	tx.committed = true
	return true
}

// Rollback 回滚事务，丢弃所有更改
func (tx *Transaction[T]) Rollback() bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return false
	}

	// 清空工作副本，标记为已提交以防止进一步操作
	tx.workingCopy = nil
	tx.committed = true
	return true
}

// IsCommitted 检查事务是否已提交
func (tx *Transaction[T]) IsCommitted() bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return tx.committed
}

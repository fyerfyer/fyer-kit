package slice

import (
	"sort"
	"sync"
)

// SafeSlice 提供并发安全的切片操作
type SafeSlice[T any] struct {
	mu    sync.RWMutex
	items []T
}

// New 创建一个新的并发安全切片
func New[T any]() *SafeSlice[T] {
	return &SafeSlice[T]{
		items: make([]T, 0),
	}
}

// NewWithCapacity 创建一个指定初始容量的并发安全切片
func NewWithCapacity[T any](capacity int) *SafeSlice[T] {
	return &SafeSlice[T]{
		items: make([]T, 0, capacity),
	}
}

// FromSlice 从现有切片创建并发安全切片
func FromSlice[T any](slice []T) *SafeSlice[T] {
	newSlice := make([]T, len(slice))
	copy(newSlice, slice)
	return &SafeSlice[T]{
		items: newSlice,
	}
}

// Append 在切片末尾添加元素
func (s *SafeSlice[T]) Append(items ...T) {
	if len(items) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, items...)
}

// Prepend 在切片开头添加元素
func (s *SafeSlice[T]) Prepend(items ...T) {
	if len(items) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(items, s.items...)
}

// Get 获取指定索引的元素，如果索引越界则返回零值和false
func (s *SafeSlice[T]) Get(index int) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T
	if index < 0 || index >= len(s.items) {
		return zero, false
	}
	return s.items[index], true
}

// GetOrDefault 获取指定索引的元素，如果索引越界则返回默认值
func (s *SafeSlice[T]) GetOrDefault(index int, defaultValue T) T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index < 0 || index >= len(s.items) {
		return defaultValue
	}
	return s.items[index]
}

// Set 设置指定索引的元素值，如果索引越界则返回false
func (s *SafeSlice[T]) Set(index int, value T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.items) {
		return false
	}
	s.items[index] = value
	return true
}

// Delete 删除指定索引的元素，如果索引越界则返回false
func (s *SafeSlice[T]) Delete(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.items) {
		return false
	}

	// 删除元素
	s.items = append(s.items[:index], s.items[index+1:]...)
	return true
}

// DeleteRange 删除指定范围的元素，如果范围无效则返回false
func (s *SafeSlice[T]) DeleteRange(start, end int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	length := len(s.items)
	if start < 0 || end > length || start > end {
		return false
	}

	s.items = append(s.items[:start], s.items[end:]...)
	return true
}

// GetMany 获取多个索引的元素，返回元素切片和是否全部成功
func (s *SafeSlice[T]) GetMany(indices []int) ([]T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(indices) == 0 {
		return []T{}, true
	}

	result := make([]T, len(indices))
	allSuccess := true

	for i, idx := range indices {
		if idx < 0 || idx >= len(s.items) {
			var zero T
			result[i] = zero
			allSuccess = false
		} else {
			result[i] = s.items[idx]
		}
	}

	return result, allSuccess
}

// BatchUpdate 批量更新多个索引的元素，提供更高效的批量更新操作
// 参数 updates 是一个 map，键为索引，值为要设置的新值
// 返回成功更新的元素数量和是否全部成功
func (s *SafeSlice[T]) BatchUpdate(updates map[int]T) (int, bool) {
	if len(updates) == 0 {
		return 0, true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	succeededCount := 0
	for idx, value := range updates {
		if idx >= 0 && idx < len(s.items) {
			s.items[idx] = value
			succeededCount++
		}
	}

	return succeededCount, succeededCount == len(updates)
}

// BatchDelete 批量删除多个索引的元素
// 返回成功删除的元素数量和是否全部成功
func (s *SafeSlice[T]) BatchDelete(indices []int) (int, bool) {
	if len(indices) == 0 {
		return 0, true
	}

	// 对索引进行排序，从大到小
	sortedIndices := make([]int, len(indices))
	copy(sortedIndices, indices)

	// 使用sort.Sort和自定义排序，从大到小排序
	sort.Sort(sort.Reverse(sort.IntSlice(sortedIndices)))

	s.mu.Lock()
	defer s.mu.Unlock()

	succeededCount := 0
	for _, idx := range sortedIndices {
		if idx >= 0 && idx < len(s.items) {
			// 从大到小删除，这样不会影响其他待删除元素的索引
			s.items = append(s.items[:idx], s.items[idx+1:]...)
			succeededCount++
		}
	}

	return succeededCount, succeededCount == len(indices)
}

// RangeAccess 提供对整个切片的批量访问，允许在单个锁定期间执行多个操作
// 参数 fn 是一个接收切片副本的函数，可以在其中执行读取操作
// 这个方法不会修改原始切片
func (s *SafeSlice[T]) RangeAccess(fn func([]T)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 创建副本以防止在回调中修改原始数据
	snapshot := make([]T, len(s.items))
	copy(snapshot, s.items)

	fn(snapshot)
}

// BatchOperation 提供对整个切片的批量修改操作
// 参数 fn 是一个接收并可能修改切片的函数，函数返回值表示是否应用修改
// 如果 fn 返回 true，则更新原始切片；如果返回 false，则忽略所有修改
func (s *SafeSlice[T]) BatchOperation(fn func([]T) ([]T, bool)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 创建副本以允许安全修改
	workingCopy := make([]T, len(s.items))
	copy(workingCopy, s.items)

	// 执行批量操作
	newItems, applyChanges := fn(workingCopy)
	if applyChanges {
		s.items = newItems
		return true
	}
	return false
}

// Len 返回切片的长度
func (s *SafeSlice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Cap 返回切片的容量
func (s *SafeSlice[T]) Cap() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cap(s.items)
}

// Clear 清空切片
func (s *SafeSlice[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = s.items[:0]
}

// Clone 克隆切片并返回一个新的SafeSlice
func (s *SafeSlice[T]) Clone() *SafeSlice[T] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	newItems := make([]T, len(s.items))
	copy(newItems, s.items)
	return &SafeSlice[T]{
		items: newItems,
	}
}

// ToSlice 返回内部切片的副本
func (s *SafeSlice[T]) ToSlice() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]T, len(s.items))
	copy(result, s.items)
	return result
}

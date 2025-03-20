package slice

import (
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

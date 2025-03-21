package set

import (
	"sync"
)

// ConcurrentSet 线程安全的集合实现，基于HashSet
type ConcurrentSet[T comparable] struct {
	set  *HashSet[T]
	lock sync.RWMutex
}

// NewConcurrent 创建一个新的并发安全集合
func NewConcurrent[T comparable](items ...T) Set[T] {
	return &ConcurrentSet[T]{
		set: New[T](items...).(*HashSet[T]),
	}
}

// Add 添加元素到集合中，线程安全
func (c *ConcurrentSet[T]) Add(item T) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.set.Add(item)
}

// Remove 从集合中删除元素，线程安全
func (c *ConcurrentSet[T]) Remove(item T) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.set.Remove(item)
}

// Contains 检查元素是否在集合中，线程安全
func (c *ConcurrentSet[T]) Contains(item T) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.set.Contains(item)
}

// AddAll 批量添加元素到集合中，线程安全
func (c *ConcurrentSet[T]) AddAll(items ...T) int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.set.AddAll(items...)
}

// RemoveAll 批量删除元素，线程安全
func (c *ConcurrentSet[T]) RemoveAll(items ...T) int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.set.RemoveAll(items...)
}

// RetainAll 仅保留指定元素，线程安全
func (c *ConcurrentSet[T]) RetainAll(items ...T) int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.set.RetainAll(items...)
}

// ContainsAll 检查集合是否包含所有指定元素，线程安全
func (c *ConcurrentSet[T]) ContainsAll(items ...T) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.set.ContainsAll(items...)
}

// Size 返回集合中的元素数量，线程安全
func (c *ConcurrentSet[T]) Size() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.set.Size()
}

// Clear 清空集合，线程安全
func (c *ConcurrentSet[T]) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.set.Clear()
}

// IsEmpty 检查集合是否为空，线程安全
func (c *ConcurrentSet[T]) IsEmpty() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.set.IsEmpty()
}

// ToSlice 将集合转换为切片，线程安全
func (c *ConcurrentSet[T]) ToSlice() []T {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.set.ToSlice()
}

// Union 返回与另一个集合的并集，线程安全
func (c *ConcurrentSet[T]) Union(other Set[T]) Set[T] {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.Union(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.Union(other)
}

// Intersection 返回与另一个集合的交集，线程安全
func (c *ConcurrentSet[T]) Intersection(other Set[T]) Set[T] {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.Intersection(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.Intersection(other)
}

// Difference 返回与另一个集合的差集，线程安全
func (c *ConcurrentSet[T]) Difference(other Set[T]) Set[T] {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.Difference(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.Difference(other)
}

// SymmetricDifference 返回与另一个集合的对称差集，线程安全
func (c *ConcurrentSet[T]) SymmetricDifference(other Set[T]) Set[T] {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.SymmetricDifference(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.SymmetricDifference(other)
}

// IsSubset 检查当前集合是否是另一个集合的子集，线程安全
func (c *ConcurrentSet[T]) IsSubset(other Set[T]) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.IsSubset(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.IsSubset(other)
}

// IsSuperset 检查当前集合是否是另一个集合的超集，线程安全
func (c *ConcurrentSet[T]) IsSuperset(other Set[T]) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// 如果other也是ConcurrentSet，需要避免潜在的死锁
	if otherConcurrent, ok := other.(*ConcurrentSet[T]); ok {
		otherConcurrent.lock.RLock()
		result := c.set.IsSuperset(otherConcurrent.set)
		otherConcurrent.lock.RUnlock()
		return result
	}

	return c.set.IsSuperset(other)
}

// ForEach 遍历集合中的所有元素，线程安全
// 注意：在回调函数中修改集合可能导致死锁
func (c *ConcurrentSet[T]) ForEach(f func(T) bool) {
	// 先创建一个副本，避免在遍历时持有锁
	c.lock.RLock()
	items := c.set.ToSlice()
	c.lock.RUnlock()

	// 在副本上遍历
	for _, item := range items {
		if !f(item) {
			break
		}
	}
}

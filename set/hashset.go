package set

// HashSet 无序集合实现，基于Go的map
type HashSet[T comparable] struct {
	items map[T]struct{} // 使用空结构体作为map值，节省内存
}

// New 创建一个新的HashSet
func New[T comparable](items ...T) Set[T] {
	set := &HashSet[T]{
		items: make(map[T]struct{}),
	}
	for _, item := range items {
		set.Add(item)
	}
	return set
}

// Add 添加元素到集合中
func (s *HashSet[T]) Add(item T) bool {
	_, exists := s.items[item]
	if exists {
		return false
	}
	s.items[item] = struct{}{}
	return true
}

// Remove 从集合中删除元素
func (s *HashSet[T]) Remove(item T) bool {
	_, exists := s.items[item]
	if !exists {
		return false
	}
	delete(s.items, item)
	return true
}

// Contains 检查元素是否在集合中
func (s *HashSet[T]) Contains(item T) bool {
	_, exists := s.items[item]
	return exists
}

// AddAll 批量添加元素到集合中，返回成功添加的元素数量
func (s *HashSet[T]) AddAll(items ...T) int {
	added := 0
	for _, item := range items {
		if s.Add(item) {
			added++
		}
	}
	return added
}

// RemoveAll 批量删除元素，返回成功删除的元素数量
func (s *HashSet[T]) RemoveAll(items ...T) int {
	removed := 0
	for _, item := range items {
		if s.Remove(item) {
			removed++
		}
	}
	return removed
}

// RetainAll 仅保留指定元素，删除集合中不在给定元素列表中的所有元素
// 返回被删除的元素数量
func (s *HashSet[T]) RetainAll(items ...T) int {
	// 创建临时集合存储要保留的元素
	retain := make(map[T]struct{})
	for _, item := range items {
		retain[item] = struct{}{}
	}

	// 找出需要删除的元素
	toRemove := make([]T, 0)
	for item := range s.items {
		if _, exists := retain[item]; !exists {
			toRemove = append(toRemove, item)
		}
	}

	// 删除元素
	for _, item := range toRemove {
		delete(s.items, item)
	}

	return len(toRemove)
}

// ContainsAll 检查集合是否包含所有指定元素
func (s *HashSet[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !s.Contains(item) {
			return false
		}
	}
	return true
}

// Size 返回集合中的元素数量
func (s *HashSet[T]) Size() int {
	return len(s.items)
}

// Clear 清空集合
func (s *HashSet[T]) Clear() {
	s.items = make(map[T]struct{})
}

// IsEmpty 检查集合是否为空
func (s *HashSet[T]) IsEmpty() bool {
	return len(s.items) == 0
}

// ToSlice 将集合转换为切片
func (s *HashSet[T]) ToSlice() []T {
	result := make([]T, 0, len(s.items))
	for item := range s.items {
		result = append(result, item)
	}
	return result
}

// Union 返回与另一个集合的并集
func (s *HashSet[T]) Union(other Set[T]) Set[T] {
	result := New[T]()
	// 添加本集合中的所有元素
	s.ForEach(func(item T) bool {
		result.Add(item)
		return true
	})
	// 添加另一个集合中的所有元素
	other.ForEach(func(item T) bool {
		result.Add(item)
		return true
	})
	return result
}

// Intersection 返回与另一个集合的交集
func (s *HashSet[T]) Intersection(other Set[T]) Set[T] {
	result := New[T]()
	s.ForEach(func(item T) bool {
		if other.Contains(item) {
			result.Add(item)
		}
		return true
	})
	return result
}

// Difference 返回与另一个集合的差集 (s - other)
func (s *HashSet[T]) Difference(other Set[T]) Set[T] {
	result := New[T]()
	s.ForEach(func(item T) bool {
		if !other.Contains(item) {
			result.Add(item)
		}
		return true
	})
	return result
}

// SymmetricDifference 返回与另一个集合的对称差集
func (s *HashSet[T]) SymmetricDifference(other Set[T]) Set[T] {
	// 对称差集 = (A ∪ B) - (A ∩ B)
	union := s.Union(other)
	intersection := s.Intersection(other)
	return union.Difference(intersection)
}

// IsSubset 检查当前集合是否是另一个集合的子集
func (s *HashSet[T]) IsSubset(other Set[T]) bool {
	if s.Size() > other.Size() {
		return false
	}

	for item := range s.items {
		if !other.Contains(item) {
			return false
		}
	}
	return true
}

// IsSuperset 检查当前集合是否是另一个集合的超集
func (s *HashSet[T]) IsSuperset(other Set[T]) bool {
	return other.IsSubset(s)
}

// ForEach 遍历集合中的所有元素
func (s *HashSet[T]) ForEach(f func(T) bool) {
	for item := range s.items {
		if !f(item) {
			break
		}
	}
}

package set

import (
	"sort"
)

// SortedSet 有序集合实现，按元素值排序
type SortedSet[T comparable] struct {
	items    map[T]struct{}    // 用于O(1)查找
	elements []T               // 已排序的元素切片
	less     func(a, b T) bool // 比较函数
}

// NewSorted 创建一个新的有序集合，使用默认排序
// 仅支持内置可比较类型
func NewSorted[T comparable](items ...T) Set[T] {
	// 创建一个默认的比较函数
	return NewSortedWithComparator(defaultLess[T], items...)
}

// NewSortedWithComparator 创建一个新的有序集合，使用自定义比较函数
func NewSortedWithComparator[T comparable](less func(a, b T) bool, items ...T) Set[T] {
	set := &SortedSet[T]{
		items:    make(map[T]struct{}),
		elements: make([]T, 0),
		less:     less,
	}
	for _, item := range items {
		set.Add(item)
	}
	return set
}

// 默认比较函数，使用类型断言处理常见类型
func defaultLess[T comparable](a, b T) bool {
	// 尝试类型断言，处理常见可比较类型
	switch v1 := any(a).(type) {
	case int:
		return v1 < any(b).(int)
	case int8:
		return v1 < any(b).(int8)
	case int16:
		return v1 < any(b).(int16)
	case int32:
		return v1 < any(b).(int32)
	case int64:
		return v1 < any(b).(int64)
	case uint:
		return v1 < any(b).(uint)
	case uint16:
		return v1 < any(b).(uint16)
	case uint32:
		return v1 < any(b).(uint32)
	case uint64:
		return v1 < any(b).(uint64)
	case float32:
		return v1 < any(b).(float32)
	case float64:
		return v1 < any(b).(float64)
	case string:
		return v1 < any(b).(string)
	case byte:
		return v1 < any(b).(byte)
	default:
		// 对于其他类型，我们无法直接比较，默认返回false
		// 这将导致元素按添加顺序排列
		return false
	}
}

// Add 添加元素到集合中并保持排序
func (s *SortedSet[T]) Add(item T) bool {
	if _, exists := s.items[item]; exists {
		return false
	}

	// 添加到map
	s.items[item] = struct{}{}

	// 使用二分查找找到新元素的插入位置
	insertPos := sort.Search(len(s.elements), func(i int) bool {
		return s.less(item, s.elements[i])
	})

	// 在正确位置插入元素
	s.elements = append(s.elements, item) // 先添加到末尾
	if insertPos < len(s.elements)-1 {    // 如果不是最大的元素
		// 将新元素移动到正确位置
		copy(s.elements[insertPos+1:], s.elements[insertPos:len(s.elements)-1])
		s.elements[insertPos] = item
	}

	return true
}

// Remove 从集合中删除元素
func (s *SortedSet[T]) Remove(item T) bool {
	if _, exists := s.items[item]; !exists {
		return false
	}

	// 从map中移除
	delete(s.items, item)

	// 从已排序切片中找到并移除
	pos := -1
	for i, v := range s.elements {
		if v == item {
			pos = i
			break
		}
	}

	if pos >= 0 {
		// 从已排序切片中移除
		s.elements = append(s.elements[:pos], s.elements[pos+1:]...)
	}

	return true
}

// Contains 检查元素是否在集合中
func (s *SortedSet[T]) Contains(item T) bool {
	_, exists := s.items[item]
	return exists
}

// Size 返回集合中的元素数量
func (s *SortedSet[T]) Size() int {
	return len(s.items)
}

// Clear 清空集合
func (s *SortedSet[T]) Clear() {
	s.items = make(map[T]struct{})
	s.elements = make([]T, 0)
}

// IsEmpty 检查集合是否为空
func (s *SortedSet[T]) IsEmpty() bool {
	return len(s.items) == 0
}

// ToSlice 将集合转换为有序切片
func (s *SortedSet[T]) ToSlice() []T {
	result := make([]T, len(s.elements))
	copy(result, s.elements)
	return result
}

// Union 返回与另一个集合的并集
func (s *SortedSet[T]) Union(other Set[T]) Set[T] {
	result := NewSortedWithComparator(s.less)

	// 添加本集合中的所有元素
	for _, item := range s.elements {
		result.Add(item)
	}

	// 添加另一个集合中的所有元素
	other.ForEach(func(item T) bool {
		result.Add(item)
		return true
	})

	return result
}

// Intersection 返回与另一个集合的交集
func (s *SortedSet[T]) Intersection(other Set[T]) Set[T] {
	result := NewSortedWithComparator(s.less)

	for _, item := range s.elements {
		if other.Contains(item) {
			result.Add(item)
		}
	}

	return result
}

// Difference 返回与另一个集合的差集 (s - other)
func (s *SortedSet[T]) Difference(other Set[T]) Set[T] {
	result := NewSortedWithComparator(s.less)

	for _, item := range s.elements {
		if !other.Contains(item) {
			result.Add(item)
		}
	}

	return result
}

// SymmetricDifference 返回与另一个集合的对称差集
func (s *SortedSet[T]) SymmetricDifference(other Set[T]) Set[T] {
	// 对称差集 = (A ∪ B) - (A ∩ B)
	union := s.Union(other)
	intersection := s.Intersection(other)
	return union.Difference(intersection)
}

// IsSubset 检查当前集合是否是另一个集合的子集
func (s *SortedSet[T]) IsSubset(other Set[T]) bool {
	if s.Size() > other.Size() {
		return false
	}

	for _, item := range s.elements {
		if !other.Contains(item) {
			return false
		}
	}
	return true
}

// IsSuperset 检查当前集合是否是另一个集合的超集
func (s *SortedSet[T]) IsSuperset(other Set[T]) bool {
	return other.IsSubset(s)
}

// ForEach 遍历集合中的所有元素，按排序顺序
func (s *SortedSet[T]) ForEach(f func(T) bool) {
	for _, item := range s.elements {
		if !f(item) {
			break
		}
	}
}

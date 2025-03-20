package slice

// Iterator 提供安全的切片遍历操作
type Iterator[T any] struct {
	slice *SafeSlice[T]
}

// NewIterator 创建一个新的迭代器
func NewIterator[T any](slice *SafeSlice[T]) *Iterator[T] {
	return &Iterator[T]{
		slice: slice,
	}
}

// ForEach 安全地遍历所有元素并执行回调函数
func (it *Iterator[T]) ForEach(fn func(index int, item T)) {
	// 获取切片的快照以避免遍历时的锁争用
	snapshot := it.slice.ToSlice()
	for i, item := range snapshot {
		fn(i, item)
	}
}

// Find 查找满足条件的第一个元素
// 如果找到则返回元素和true，否则返回零值和false
func (it *Iterator[T]) Find(predicate func(item T) bool) (T, bool) {
	snapshot := it.slice.ToSlice()
	var zero T

	for _, item := range snapshot {
		if predicate(item) {
			return item, true
		}
	}

	return zero, false
}

// Filter 创建一个新的SafeSlice，只包含满足条件的元素
func (it *Iterator[T]) Filter(predicate func(item T) bool) *SafeSlice[T] {
	snapshot := it.slice.ToSlice()
	result := New[T]()

	for _, item := range snapshot {
		if predicate(item) {
			result.Append(item)
		}
	}

	return result
}

// Any 判断是否存在满足条件的元素
func (it *Iterator[T]) Any(predicate func(item T) bool) bool {
	_, found := it.Find(predicate)
	return found
}

// All 判断是否所有元素都满足条件
func (it *Iterator[T]) All(predicate func(item T) bool) bool {
	snapshot := it.slice.ToSlice()

	for _, item := range snapshot {
		if !predicate(item) {
			return false
		}
	}

	return true
}

// Map 创建一个函数，将切片元素映射到另一种类型
// 这是一个工具函数，而不是 Iterator 的方法
func Map[T, R any](slice *SafeSlice[T], mapper func(T) R) *SafeSlice[R] {
	snapshot := slice.ToSlice()
	result := New[R]()

	for _, item := range snapshot {
		result.Append(mapper(item))
	}

	return result
}

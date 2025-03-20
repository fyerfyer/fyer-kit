package slice

// CompareAndSwap 原子地比较并交换指定索引的元素
// 只有当当前值与预期值相等时才设置新值
// 返回是否成功交换
func (s *SafeSlice[T]) CompareAndSwap(index int, expected, new T, equals func(a, b T) bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.items) {
		return false
	}

	current := s.items[index]
	if equals(current, expected) {
		s.items[index] = new
		return true
	}
	return false
}

// AtomicUpdate 原子地更新指定索引的元素
// 传入一个转换函数，该函数接收当前值并返回新值
// 返回是否成功更新
func (s *SafeSlice[T]) AtomicUpdate(index int, updateFn func(current T) T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.items) {
		return false
	}

	current := s.items[index]
	s.items[index] = updateFn(current)
	return true
}

// CompareAndDelete 原子地比较并删除指定索引的元素
// 只有当当前值与预期值相等时才删除
// 返回是否成功删除
func (s *SafeSlice[T]) CompareAndDelete(index int, expected T, equals func(a, b T) bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.items) {
		return false
	}

	current := s.items[index]
	if equals(current, expected) {
		s.items = append(s.items[:index], s.items[index+1:]...)
		return true
	}
	return false
}

// GetOrCompute 获取指定索引的元素，如果索引越界则计算并添加新元素
// 返回最终获取或计算的元素和一个布尔值表示是否是新计算的
func (s *SafeSlice[T]) GetOrCompute(index int, compute func() T) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果索引有效，直接返回现有元素
	if index >= 0 && index < len(s.items) {
		return s.items[index], false
	}

	// 如果索引是下一个位置（末尾），计算并添加
	if index == len(s.items) {
		newItem := compute()
		s.items = append(s.items, newItem)
		return newItem, true
	}

	// 索引无效
	var zero T
	return zero, false
}

// FindAndUpdate 查找满足条件的第一个元素并更新它
// 返回是否找到并更新了元素
func (s *SafeSlice[T]) FindAndUpdate(predicate func(T) bool, updateFn func(T) T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.items {
		if predicate(item) {
			s.items[i] = updateFn(item)
			return true
		}
	}
	return false
}

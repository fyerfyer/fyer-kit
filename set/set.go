package set

type Set[T comparable] interface {
	// 基本操作
	Add(item T) bool      // 添加元素，如果元素已存在返回false，否则返回true
	Remove(item T) bool   // 删除元素，如果元素不存在返回false，否则返回true
	Contains(item T) bool // 检查元素是否存在
	Size() int            // 返回集合大小
	Clear()               // 清空集合
	IsEmpty() bool        // 检查集合是否为空
	ToSlice() []T         // 将集合转换为切片

	// 批量操作
	AddAll(items ...T) int       // 批量添加元素，返回成功添加的元素数量
	RemoveAll(items ...T) int    // 批量删除元素，返回成功删除的元素数量
	RetainAll(items ...T) int    // 仅保留指定元素，返回被删除的元素数量
	ContainsAll(items ...T) bool // 检查是否包含所有指定元素

	// 集合运算
	Union(other Set[T]) Set[T]               // 并集
	Intersection(other Set[T]) Set[T]        // 交集
	Difference(other Set[T]) Set[T]          // 差集
	SymmetricDifference(other Set[T]) Set[T] // 对称差集
	IsSubset(other Set[T]) bool              // 判断是否为子集
	IsSuperset(other Set[T]) bool            // 判断是否为超集

	// 迭代
	ForEach(f func(T) bool) // 遍历集合，返回false可停止遍历
}

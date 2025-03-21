package set

import (
	"testing"
)

func TestSortedSet_ValueOrder(t *testing.T) {
	// 创建有序集合，无序添加数字
	set := NewSorted[int](3, 1, 4, 1, 5, 9, 2, 6)

	// 应该按照值的大小排序: 1, 2, 3, 4, 5, 6, 9
	expected := []int{1, 2, 3, 4, 5, 6, 9}
	result := set.ToSlice()

	// 检查元素数量
	if len(result) != len(expected) {
		t.Errorf("SortedSet size incorrect. Expected %d, got %d", len(expected), len(result))
	}

	// 检查元素是否按值排序
	for i, v := range expected {
		if i < len(result) && result[i] != v {
			t.Errorf("Order mismatch at position %d. Expected %d, got %d", i, v, result[i])
		}
	}

	// 检查重复元素被忽略
	if set.Size() != 7 {
		t.Errorf("SortedSet should ignore duplicates. Expected size 7, got %d", set.Size())
	}
}

func TestSortedSet_StringValueOrder(t *testing.T) {
	// 创建字符串有序集合
	set := NewSorted[string]("banana", "apple", "orange", "kiwi", "apple")

	// 应该按照字符串字典顺序排序
	expected := []string{"apple", "banana", "kiwi", "orange"}
	result := set.ToSlice()

	// 检查元素数量
	if len(result) != len(expected) {
		t.Errorf("SortedSet size incorrect. Expected %d, got %d", len(expected), len(result))
	}

	// 检查元素是否按字典序排序
	for i, v := range expected {
		if i < len(result) && result[i] != v {
			t.Errorf("Order mismatch at position %d. Expected %s, got %s", i, v, result[i])
		}
	}
}

func TestSortedSet_CustomComparator(t *testing.T) {
	// 使用自定义比较函数创建集合 (降序排列)
	reverseOrder := func(a, b int) bool {
		return a > b // 降序比较
	}

	set := NewSortedWithComparator(reverseOrder, 3, 1, 4, 1, 5, 9, 2, 6)

	// 应该按照降序排列: 9, 6, 5, 4, 3, 2, 1
	expected := []int{9, 6, 5, 4, 3, 2, 1}
	result := set.ToSlice()

	// 检查元素是否按降序排序
	for i, v := range expected {
		if i < len(result) && result[i] != v {
			t.Errorf("Custom order mismatch at position %d. Expected %d, got %d", i, v, result[i])
		}
	}
}

func TestSortedSet_OperationsPreserveOrder(t *testing.T) {
	// 创建两个有序集合
	set1 := NewSorted[int](1, 7, 3, 9, 5)
	set2 := NewSorted[int](2, 5, 8, 3)

	// 测试并集操作 - 应该按升序排列
	union := set1.Union(set2).ToSlice()
	expectedUnion := []int{1, 2, 3, 5, 7, 8, 9}

	if len(union) != len(expectedUnion) {
		t.Errorf("Union size incorrect. Expected %d, got %d", len(expectedUnion), len(union))
	}

	// 检查并集中的元素顺序
	for i, v := range expectedUnion {
		if i < len(union) && union[i] != v {
			t.Errorf("Union order mismatch at position %d. Expected %d, got %d", i, v, union[i])
		}
	}

	// 测试交集操作 - 应该按升序排列
	intersection := set1.Intersection(set2).ToSlice()
	expectedIntersection := []int{3, 5}

	if len(intersection) != len(expectedIntersection) {
		t.Errorf("Intersection size incorrect. Expected %d, got %d", len(expectedIntersection), len(intersection))
	}

	// 检查交集中的元素顺序
	for i, v := range expectedIntersection {
		if i < len(intersection) && intersection[i] != v {
			t.Errorf("Intersection order mismatch at position %d. Expected %d, got %d", i, v, intersection[i])
		}
	}
}

func TestSortedSet_RemoveAndAdd(t *testing.T) {
	// 创建有序集合
	set := NewSorted[string]("apple", "banana", "cherry", "date")

	// 删除中间元素
	set.Remove("banana")
	expected1 := []string{"apple", "cherry", "date"}
	result1 := set.ToSlice()

	for i, v := range expected1 {
		if result1[i] != v {
			t.Errorf("After Remove: order mismatch at position %d. Expected %s, got %s", i, v, result1[i])
		}
	}

	// 添加新元素
	set.Add("blueberry")
	set.Add("avocado")

	// 应该按照字典顺序排列
	expected2 := []string{"apple", "avocado", "blueberry", "cherry", "date"}
	result2 := set.ToSlice()

	for i, v := range expected2 {
		if result2[i] != v {
			t.Errorf("After Add: order mismatch at position %d. Expected %s, got %s", i, v, result2[i])
		}
	}
}

func TestSortedSet_BatchOperations(t *testing.T) {
	// 测试批量添加
	set := NewSorted[int]()
	addedCount := set.AddAll(5, 3, 1, 4, 2, 3) // 3 重复

	if addedCount != 5 {
		t.Errorf("AddAll should return count of added elements. Expected 5, got %d", addedCount)
	}

	expected := []int{1, 2, 3, 4, 5}
	result := set.ToSlice()

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("After AddAll: order mismatch at position %d. Expected %d, got %d", i, v, result[i])
		}
	}

	// 测试批量删除
	removedCount := set.RemoveAll(1, 3, 7) // 7 不存在

	if removedCount != 2 {
		t.Errorf("RemoveAll should return count of removed elements. Expected 2, got %d", removedCount)
	}

	expected2 := []int{2, 4, 5}
	result2 := set.ToSlice()

	for i, v := range expected2 {
		if result2[i] != v {
			t.Errorf("After RemoveAll: order mismatch at position %d. Expected %d, got %d", i, v, result2[i])
		}
	}

	// 测试RetainAll
	set = NewSorted[int](1, 2, 3, 4, 5)
	removedByRetain := set.RetainAll(2, 4, 6) // 保留 2,4，6不在集合中

	if removedByRetain != 3 {
		t.Errorf("RetainAll should return count of removed elements. Expected 3, got %d", removedByRetain)
	}

	expected3 := []int{2, 4}
	result3 := set.ToSlice()

	for i, v := range expected3 {
		if result3[i] != v {
			t.Errorf("After RetainAll: order mismatch at position %d. Expected %d, got %d", i, v, result3[i])
		}
	}

	// 测试ContainsAll
	set = NewSorted[int](1, 2, 3, 4, 5)
	if !set.ContainsAll(1, 3, 5) {
		t.Errorf("ContainsAll should return true when all elements are in the set")
	}

	if set.ContainsAll(1, 3, 6) {
		t.Errorf("ContainsAll should return false when any element is not in the set")
	}
}

func TestSortedSet_BatchOperationsWithCustomOrder(t *testing.T) {
	// 使用自定义比较函数创建集合 (降序排列)
	reverseOrder := func(a, b int) bool {
		return a > b // 降序比较
	}

	set := NewSortedWithComparator(reverseOrder)
	set.AddAll(3, 5, 1, 2, 4)

	// 应该按照降序排列: 5, 4, 3, 2, 1
	expected := []int{5, 4, 3, 2, 1}
	result := set.ToSlice()

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("After AddAll with custom order: mismatch at position %d. Expected %d, got %d", i, v, result[i])
		}
	}

	// 批量删除后保持排序
	set.RemoveAll(3, 1)
	expected2 := []int{5, 4, 2}
	result2 := set.ToSlice()

	for i, v := range expected2 {
		if result2[i] != v {
			t.Errorf("After RemoveAll with custom order: mismatch at position %d. Expected %d, got %d", i, v, result2[i])
		}
	}
}

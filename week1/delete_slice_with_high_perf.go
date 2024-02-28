package main

import (
	"fmt"
	"reflect"
)

// RemoveAtIndex 删除切片中指定下标的元素
// 泛型方法，使用反射实现
func RemoveAtIndex(slice interface{}, index int) interface{} {
	sliceValue := reflect.ValueOf(slice)

	if sliceValue.Kind() != reflect.Slice {
		panic("Invalid argument type. Expecting a slice.")
	}

	// 创建新的切片
	newSlice := reflect.MakeSlice(sliceValue.Type(), sliceValue.Len()-1, sliceValue.Len()-1)

	// 拷贝原切片中除指定下标元素外的所有元素到新切片
	reflect.Copy(newSlice, sliceValue.Slice(0, index))
	reflect.Copy(newSlice.Slice(index, newSlice.Len()), sliceValue.Slice(index+1, sliceValue.Len()))

	return newSlice.Interface()
}

func main() {

	// 删除下标为2的元素
	newIntSlice := RemoveAtIndex(intSlice, 2).([]int)
	fmt.Println("Modified slice:", newIntSlice)

	// 示例使用泛型方法
	strSlice := []string{"apple", "banana", "cherry"}
	fmt.Println("Original slice:", strSlice)

	// 删除下标为1的元素
	newStrSlice := RemoveAtIndex(strSlice, 1).([]string)
	fmt.Println("Modified slice:", newStrSlice)
}

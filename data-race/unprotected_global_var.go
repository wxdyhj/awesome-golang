package main

import (
	"fmt"
	"sync"
)

var (
	num2Name = make(map[int]string) // key: number, value: name
)

func SetName(num int, name string) {
	num2Name[num] = name
}

func GetName(num int) string {
	return num2Name[num]
}

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		SetName(1, "Liu Bei")
	}()
	go func() {
		defer wg.Done()
		fmt.Println(GetName(1))
	}()
	wg.Wait()
}

package main

import (
	"fmt"
	"sync"
)

var (
	num2Name = make(map[int]string) // key: number, value: name
	lock     = new(sync.RWMutex)    // read and write lock
)

func SetName(num int, name string) {
	lock.Lock()
	defer lock.Unlock()
	num2Name[num] = name
}

func GetName(num int) string {
	lock.RLock()
	defer lock.RUnlock()
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

package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	n := 5
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			fmt.Println("i", i) // it is not the 'i' you are looking for.
			wg.Done()
		}()
	}
	wg.Wait()
}

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
		go func(j int) {
			fmt.Println("j", j) // read local copy of the loop counter
			wg.Done()
		}(i)
	}
	wg.Wait()
}

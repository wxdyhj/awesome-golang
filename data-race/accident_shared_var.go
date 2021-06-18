package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	data := []byte("hello golang")
	res := make(chan error, 2)
	f1, err := os.Create("file1")
	fmt.Printf("pointer of err from file1: %p\n", &err)
	if err != nil {
		res <- err
	} else {
		go func() {
			// This err is shared with the main goroutine, so the write races with the write below.
			_, err = f1.Write(data)
			res <- err
			f1.Close()
		}()
	}

	f2, err := os.Create("file2")
	fmt.Printf("pointer of err from file2: %p\n", &err)
	if err != nil {
		res <- err
	} else {
		go func() {
			_, err = f2.Write(data)
			res <- err
			f2.Close()
		}()
	}

	// receive the error message
	go func() {
		for err := range res {
			fmt.Println("err", err)
		}
	}()

	time.Sleep(1 * time.Second) // wait the goroutine to receive the err
}

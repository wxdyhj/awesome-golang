# Data Race Detector for Golang

主要介绍 race detector 的用法， golang 中常见的 data race 案例以及消除 race 的方案

## 1 race detector 简介
* golang 代码发生 data race 的条件：多个（大于或等于2 个） goroutine 同时对同一个变量进行读/写，并且至少有一个 goroutine 进行写操作。如果所有 goroutine 都只是进行读操作，那将不会构成数据争用。
* 为协助定位 data race 的 bug, Golang 提供了内置的 data race 检测工具。用法很简单，只需在 go 命令加上 -race 参数，官网文档提示支持以下 4 种方法，选择一种适合自己的就行。
```
go run -race mysrc.go // to run the source file
go build -race mycmd // to build the command
go test -race mypkg // to test the package
go install -race mypkg // to install the package
```
* 但其实 ```go build -race``` 最终编译出的二进制包，也可用于检测是否存在数据竞态
* race detector 只能找出运行时的数据竞态，所以未执行代码的数据竞态无法被检测出。测试用例很难覆盖全所有逻辑代码，在现实工作负载情况下，运行用 -race 参数编译出的二进制文件，可以找出更多的数据竞态问题

## 2 常见的 data race 样例
### 2.1 循环计数变量
* 测试源码 [loop_counter.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/loop_counter.go)
```
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
```

* 竞态检测

```
[whoareyou@data-race]$ go run -race loop_counter.go
==================
WARNING: DATA RACE
Read at 0x00c000018080 by goroutine 8:
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:14 +0x3c

Previous write at 0x00c000018080 by main goroutine:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:12 +0x104

Goroutine 8 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:13 +0xdc
==================
i 2
==================
WARNING: DATA RACE
Read at 0x00c000018080 by goroutine 7:
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:14 +0x3c

Previous write at 0x00c000018080 by main goroutine:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:12 +0x104

Goroutine 7 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/loop_counter.go:13 +0xdc
==================
i 2
i 5
i 5
i 5
Found 2 data race(s)
exit status 66
```
* 原因分析
    * 用于控制退出循环的变量 i, 对 i 执行自增操作, 有写操作；循环内部，起 5 个 goroutine, 每个 goroutine 均输出 i, 存在读操作；满足 data race 发生的条件
    * 执行 ```go run loop_counter.go``` 大概率会输出 55555，而不是 01234
    * 在开启数据竞态检测的情况下，每次输出的结果几乎不同，这是因为存在数据并发读写，程序行为不可预测


* 消除竞态 [loop_counter_no_race.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/loop_counter_no_race.go)
```
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

```

* 消除分析
    * 循环内部各 goroutine 所读的 j, 均是循环控制变量 i 的一份拷贝，不存在竞态问题
    * 上述程序会输出期望得到的 01234

### 2.2 意外共享变量
* 测试源码 [accident_shared_var.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/accident_shared_var.go)
```
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
```
* 竞态检测
```
[whoareyou@data-race]$ go run -race accident_shared_var.go 
pointer of err from file1: 0xc0000a01e0
pointer of err from file2: 0xc0000a01e0
err <nil>
==================
WARNING: DATA RACE
Write at 0x00c0000a01e0 by goroutine 7:
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/accident_shared_var.go:19 +0x94

Previous write at 0x00c0000a01e0 by main goroutine:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/accident_shared_var.go:25 +0x2f2

Goroutine 7 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/accident_shared_var.go:17 +0x528
==================
err <nil>
Found 1 data race(s)
exit status 66
```
* 原因分析
    * 内层 goroutine 与 外层 goroutine 使用同一个 err 变量，两个 goroutine 并发对 err 执行写操作
* 消除竞态
```
			_, err := f1.Write(data)
			
			_, err := f2.Write(data)
```
* 消除分析
    * 只需改两行代码，注意 := 的使用，此时内层 goroutine 的 err 是新变量

### 2.3 未受保护的全局变量
* 测试源码 [unprotected_global_var.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/unprotected_global_var.go)

```
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
```
* 竞态检测
```
[whoareyou@data-race]$ go run -race unprotected_global_var.go 
==================
WARNING: DATA RACE
Read at 0x00c00011c180 by goroutine 8:
  runtime.mapaccess1_fast64()
      /usr/local/go1.15.4/src/runtime/map_fast64.go:12 +0x0
  main.GetName()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:17 +0xb3
  main.main.func2()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:29 +0xd9

Previous write at 0x00c00011c180 by goroutine 7:
  runtime.mapassign_fast64()
      /usr/local/go1.15.4/src/runtime/map_fast64.go:92 +0x0
  main.SetName()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:13 +0xa4
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:25 +0x6d

Goroutine 8 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:27 +0xc8

Goroutine 7 (finished) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:23 +0xa6
==================
==================
WARNING: DATA RACE
Read at 0x00c00008e048 by goroutine 8:
  main.GetName()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:17 +0xc9
  main.main.func2()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:29 +0xd9

Previous write at 0x00c00008e048 by goroutine 7:
  main.SetName()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:13 +0xb9
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:25 +0x6d

Goroutine 8 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:27 +0xc8

Goroutine 7 (finished) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_global_var.go:23 +0xa6
==================
Liu Bei
Found 2 data race(s)
exit status 66
```
* 原因分析
    * 上述代码发生竞态的条件：SetName 和 GetName 访问的是同一个 map num2Name
    * 并发读写同一个 map
* 消除竞态[protected_global_var.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/protected_global_var.go)
```
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

```
* 消除分析
    * 解决思路：使用读写锁控制并发，访问 map 前加锁

### 2.4 未受保护的私有变量
* 测试源码 [unprotected_primitive_var.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/unprotected_primitive_var.go)
```
package main

import (
	"fmt"
	"os"
	"time"
)

type Watchdog struct{ last int64 }

func (w *Watchdog) KeepAlive() {
	w.last = time.Now().UnixNano() // First conflicting access.
}

func (w *Watchdog) Start() {
	go func() {
		for {
			time.Sleep(time.Second)
			// Second conflicting access.
			if w.last < time.Now().Add(-10*time.Second).UnixNano() {
				fmt.Println("No keepalives for 10 seconds. Dying.")
				os.Exit(1)
			}
		}
	}()
}

func main() {
	watchDog := Watchdog{}
	go watchDog.KeepAlive()
	watchDog.Start()
	time.Sleep(10 * time.Second) // wait a moment
}
```
* 竞态检测
```
[whoareyou@data-race]$ go run -race unprotected_primitive_var.go
==================
WARNING: DATA RACE
Read at 0x00c00013e008 by goroutine 8:
  main.(*Watchdog).Start.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_primitive_var.go:20 +0xce

Previous write at 0x00c00013e008 by goroutine 7:
  main.(*Watchdog).KeepAlive()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_primitive_var.go:12 +0xa4

Goroutine 8 (running) created at:
  main.(*Watchdog).Start()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_primitive_var.go:16 +0x4c
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_primitive_var.go:31 +0x88

Goroutine 7 (finished) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unprotected_primitive_var.go:30 +0x7a
==================
Found 1 data race(s)
exit status 66
```
* 原因分析
    * KeepAlive 和 Start 函数均访问了私有变量 last
    * 当多个 goroutine 并发调用 KeepAlive 和 Start 函数时，就会发生竞态
* 消除竞态
    * 常规方法：使用 channel 或者 锁。
    * 当然，也可以使用 sync/automic 包来实现无锁行为
```
type Watchdog struct{ last int64 }

func (w *Watchdog) KeepAlive() {
	atomic.StoreInt64(&w.last, time.Now().UnixNano())
}

func (w *Watchdog) Start() {
	go func() {
		for {
			time.Sleep(time.Second)
			if atomic.LoadInt64(&w.last) < time.Now().Add(-10*time.Second).UnixNano() {
				fmt.Println("No keepalives for 10 seconds. Dying.")
				os.Exit(1)
			}
		}
	}()
}
```

### 2.5 异步的 channel 写和关闭操作
* 测试源码 [unsynchronized_channel.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/unsynchronized_channel.go)
```
package main

func main() {
	c := make(chan struct{}) // or buffered channel

	// The race detector cannot derive the happens before relation
	// for the following send and close operations. These two operations
	// are unsynchronized and happen concurrently.
	go func() { c <- struct{}{} }()
	close(c)
}
```
* 竞态检测
```
[whoareyou@data-race]$ go run -race unsynchronized_channel.go 
==================
WARNING: DATA RACE
Read at 0x00c00005e070 by goroutine 6:
  runtime.chansend()
      /usr/local/go1.15.4/src/runtime/chan.go:158 +0x0
  main.main.func1()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unsynchronized_channel.go:9 +0x44

Previous write at 0x00c00005e070 by main goroutine:
  runtime.closechan()
      /usr/local/go1.15.4/src/runtime/chan.go:357 +0x0
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unsynchronized_channel.go:10 +0x74

Goroutine 6 (running) created at:
  main.main()
      /Users/whoareyou/code/go/src/github.com/wxdyhj/awesome-golang/data-race/unsynchronized_channel.go:9 +0x66
==================
Found 1 data race(s)
exit status 66
```
* 原因分析
    * 存在对同一个 channel 同时写和关闭的操作
    * According to the Go memory model, a send on a channel happens before the corresponding receive from that channel completes.
* 竞态消除 [synchronized_channel.go](https://github.com/wxdyhj/awesome-golang/blob/master/data-race/synchronized_channel.go)
```
package main

func main() {
	c := make(chan struct{}) // or buffered channel

	// The race detector cannot derive the happens before relation
	// for the following send and close operations. These two operations
	// are unsynchronized and happen concurrently.
	go func() { c <- struct{}{} }()
	<-c // receive operation can guarantee the send is done
	close(c)
}
```
* 消除分析
    * To synchronize send and close operations, use a receive operation that guarantees the send is done before the close
    * 读 channel 的操作能够保证 写 channel 的动作已完成

## 3 race detector 的高级玩法
可不掌握，有需求时，查阅 [Options](https://golang.org/doc/articles/race_detector#Options) 即可
* 环境变量 ```GORACE``` 支持设置 race detector 的选项，设置格式如下：
```
GORACE="option1=val1 option2=val2"
```
* 支持的选项
    * log_path: 检测报告输出文件，默认 stderr，若指定 log_path，检测结果将会输出到名为```log_path.pid```的文件中
    * exitcode: 当检测到 data race 时程序的退出状态码，默认 66
    * strip_path_prefix: 检测结果报告 data race 位置时，跳过的文件路径前缀，默认 ""
    * history_size: 每个 goroutine 可访问的内存历史大小为```32K * 2**history_size```，默认 1，增加该值的大小，可解决 "failed to restore the stack" 的问题
    * halt_on_error: 当报告第一个 data race 时，程序是否退出，默认 0，即默认不退出，设置成 非0，程序检测到 data race 就会退出
    * atexit_sleep_ms: 在 main 函数退出前的睡眠时间，单位毫秒，默认 1000（这么做的目的是，保证 race detector 有足够的时间输出监测报告。仅是个人理解，未在官网找到证据）
* 举例
```
GORACE="log_path=/tmp/race/report strip_path_prefix=/my/go/sources/" go run -race main.go
```
其实就是设置了 GORACE 的环境变量，若要清除 race detector 的设置，执行 ```unset GORACE``` 即可清除 GORACE 环境变量

## 4 小结
* 养成对 golang 项目做静态监测的习惯
* 并发读写 bool, int, int64 等类型变量，也会产生 data race，一定要消除
* 用于线上环境运行的二进制文件，编译时一定不要使用 -race 参数
* 竞态检测开销
    * 内存增加 5-10 倍
    * 运行时间增加 2-20 倍
* 非原子操作内存引起的 data race，会带来难以排查的 bug
* race detector 支持的操作系统: linux/amd64, linux/ppc64le, linux/arm64, freebsd/amd64, netbsd/amd64, darwin/amd64, darwin/arm64, and windows/amd64
* race detector 会为每个 defer 和 recover 语句申请额外的 8 字节内存空间，所在 goroutine 退出时才会被回收。所以，如果程序中存在不退出的 goroutine，并且循环调用 defer 或 recover，那程序使用的内存将会无限增长
* 在 go 内存模型中，存在 race 的程序的行为是未定义的，理论上可能出现任何情况

## 5 参考资料
* [Data Race Detector](https://golang.org/doc/articles/race_detector.html)
* [
Introducing the Go Race Detector](https://blog.golang.org/race-detector)
* [The Go Memory Model](https://golang.org/ref/mem)

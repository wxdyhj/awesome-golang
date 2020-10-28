# Diagnostics

[toc]

## 1 简介
* 对 cpu 采样：```go tool pprof http://your-prd-addr:port/debug/pprof/profile?seconds=30```
* 对内存采样：```go tool pprof http://your-prd-addr:port/debug/pprof/heap```

golang 问题定位工具可分为如下 9 种：
1. allocs: A sampling of all past memory allocations
2. block: Stack traces that led to blocking on synchronization primitives
3. cmdline: The command line invocation of the current program
4. goroutine: Stack traces of all current goroutines
5. heap: A sampling of memory allocations of live objects. You can specify the gc GET parameter to run GC before taking the heap sample.
6. mutex: Stack traces of holders of contended mutexes
7. profile: CPU profile. You can specify the duration in the seconds GET parameter. After you get the profile file, use the go tool pprof command to investigate the profile.
8. threadcreate: Stack traces that led to the creation of new OS threads
9. trace: A trace of execution of the current program. You can specify the duration in the seconds GET parameter. After you get the trace file, use the go tool trace command to investigate the trace.


## 2 profiling 的生成与分析
### 2.1 概述
注意：若要开启 web，需安装 Graphviz。

如果开启 cpu profile，程序每秒大约采样 100 次（官网原文：When CPU profiling is enabled, the Go program stops about 100 times per second and records a sample consisting of the program counters on the currently executing goroutine's stack.），显然，开启 cpu profile 会降低程序性能。

主要用途：用于定位耗时高或者频繁调用的函数。
由 package “runtime/pprof” 提供：
* cpu：cpu profile 用于判断耗时高的函数调用
* heap：heap profile 提供内存分配采样报告，监控当前和历史内存使用情况，用于判断是否内存泄露
* threadcreate：用于监控创建操作系统线程数
* block：用于展示 goroutine 阻塞在哪个位置，block profile 默认不开启，使用runtime.SetBlockProfileRate 开启
* mutex：检测锁竞争，如果怀疑因为锁竞争导致 cpu 未得到充分利用，可用 mutex 检测，默认不开启，可通过 runtime.SetMutexProfileFraction 开启。

### 2.2 生成 profile 文件
#### 2.2.1 程序主动生成 profile 文件
特性：在特定时机生成 cpu 和 内存 profile 文件，可离线分析 profile 文件，定位问题。
根据官网 https://golang.org/pkg/runtime/pprof/ 整理如下：
若开启 cpu 资源采样，golang 默认采样频率是每秒 100 次。
```
// main.go 开启的样例代码如下：
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
)

func main() {
	cpuProfile := flag.String("cpu", "", "write cpu profile to file")
	memProfile := flag.String("mem", "", "whrite mem profile to file")
	flag.Parse()
	if *cpuProfile != "" { // cpu info will be sampled during the lifetime of the program
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalln(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalln("could not start cpu profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	var ch chan bool
	ch = make(chan bool)
	go calc(ch)
	recv := <-ch
	fmt.Printf("receive %v from channel, existed\n", recv)
	writeMemInfo(memProfile)
}

// writeMemInfo writes mem info to file before the program existed
func writeMemInfo(memProfile *string) {
	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			log.Fatalln(err)
		}
		defer f.Close() // unhandled error
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatalln("could not write memory profile: ", err)
		}
	}
}

// fibo returns number of fibonacci series at the n position
func fibo(n int) int {
	if n <= 0 {
		return -1
	}
	if n == 1 || n == 2 {
		return 1
	}
	return fibo(n-1) + fibo(n-2)
}

// calc will consume much cpu and mem
func calc(ch chan bool) {
	m1 := make(map[string]string, 100000)
	m1["a"] = "1"
	fmt.Println(fibo(43))
	fmt.Println(len(m1))
	ch <- true
}
```

* 编译：执行 go build main.go，生成可执行文件 main
* 运行程序：./main -cpu cpu.prof -mem mem.prof
* 使用 pprof 分析内存 profile 文件：```go tool pprof main mem.prof``` 或者 ```go tool pprof mem.prof```
* 使用 pprof 分析 cpu profile 文件：```go tool pprof main cpu.prof``` 或者 ```go tool pprof cpu.prof```
* 在交互式界面输入 web 命令，即可打开图形界面。(需要安装 graphviz)

#### 2.2.2 http 服务采样
根据 https://golang.org/pkg/net/http/pprof/ 整理如下：
// main.go 开启 pprof 的方法如下：如果为了分析 cpu，内存使用情况，需起 goroutite，模拟消耗资源的动作。

```
package main

import (
   "log"
   "net/http"
   _ "net/http/pprof" // important!
)

func main() {
   go func() {
      log.Println(http.ListenAndServe("0.0.0.0:9002", nil))
   }()
   select {} // just to block
}
```

* 以上开启方式是在程序未开启 http 服务的情况下的一种开启方式。若使用 go 原生的 http 服务，只需 import _ "net/http/pprof" 即可开启。
* 运行程序后，再浏览器打开 https://ip:port/debug/pprof 即可。
* 如果想生成资源使用情况分布图，方法如下：
    * 对 cpu 采样：```go tool pprof http://127.0.0.1:9002/debug/pprof/profile?seconds=2```
    * 对内存采样：```go tool pprof http://your-prd-addr:port/debug/pprof/heap```
    * 对 cpu 采样(自动打开浏览器，占用指定端口1234)：```go tool pprof -http=:1234 http://your-prd-addr:port/debug/pprof/profile?seconds=30```
    * 对内存采样(自动打开浏览器，占用指定端口1235)：```go tool pprof -http=:1235 http://your-prd-addr:port/debug/pprof/heap```


#### 2.2.3 火焰图
执行 ```go tool pprof -http=:1234 http://your-prd-addr:port/debug/pprof/profile?seconds=30``` 或者 ```go tool pprof -http=:1234 http://your-prd-addr:port/debug/pprof/heap``` 命令，开启 web ui 后，可在左上角的 View 切换展示方式，包括：Top, Grapg, Flame Graph, Peek, Source, Disassemble

#### 2.2.4 分析比较
分析方法简介：在交互式界面输入 help 可展示所有命令
常用命令介绍：
* top：展示占用（cpu或内存）资源前几名的调用
* web：打开整个工程的所有函数调用关系图及其耗时
* web + 函数名：打开指定函数的下游调用关系及其耗时
* 比较：
    * 程序主动写采样文件：使用场景：线上，bug 随机复现。可考虑程序定期对 cpu 和 内存采样（比如：每分钟采集一次，每次生成一个文件，结合监控图，找到出现问题时间的 profile 文件，进行分析）
    * http 服务采样：适用于开发阶段，或者 bug 稳定复现的场景。当外部访问对应接口时，才进行实时采样，所以线上服务，若是内网服务，可大胆以 http 服务采样的方式打开 pprof，对接公网的服务，需考虑考虑监听端口被攻击的风险。

## 3 使用样例
### 3.1 http 服务采样
执行 go run main.go 即可
```
// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func main() {
	now := time.Now()
	fibo(36)
	fmt.Println(time.Now().Sub(now).Seconds())

	// start http server
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:9002", nil))
	}()

	ch1 := make(chan bool, 10)
	ch2 := make(chan bool, 10)
	for i := 0; i < 10000; i++ {
		ch1 <- true
		go f2(ch1)
		ch2 <- true
		go f3(ch2)
	}
	fmt.Println("finished")
}

func f2(ch chan bool) []string {
	fibo(36)
	x := make([]string, 0, 10000)
	x = append(x, "1")
	time.Sleep(1 * time.Second)
	<-ch
	return x
}

// f3 makes string slice to cost memory
func f3(ch chan bool) []int {
	fibo(36)
	x := make([]int, 0, 10000)
	x = append(x, 2)
	time.Sleep(1 * time.Second)
	<-ch
	return x
}

// fibo uses recursion to cost cpu
func fibo(n int) int {
	if n <= 0 {
		return 0
	}
	if n == 1 || n == 2 {
		return 1
	}
	return fibo(n-1) + fibo(n-2)
}
```

### 3.2 内存采样分析
```
go tool pprof http://127.0.0.1:9002/debug/pprof/heap
```
执行 top 命令后，内容如下：
```
Fetching profile over HTTP from http://127.0.0.1:9002/debug/pprof/heap
Saved profile in /Users/huangjie5/pprof/pprof.alloc_objects.alloc_space.inuse_objects.inuse_space.012.pb.gz
Type: inuse_space
Time: Oct 28, 2020 at 7:24pm (CST)
Entering interactive mode (type "help" for commands, "o" for options)
(pprof) top
Showing nodes accounting for 1745.36kB, 100% of 1745.36kB total
      flat  flat%   sum%        cum   cum%
 1192.32kB 68.31% 68.31%  1192.32kB 68.31%  main.f2
  553.04kB 31.69%   100%   553.04kB 31.69%  main.f3
(pprof)
```
* flat: 对应函数占用的内存量量(不包括子函数占用的内存)
* flat%: 对应函数占用进程总申请内存的比例(不包括子函数占用的内存)
* sum%: 累计比例，从上到下累加
* cum: 对应函数占用的内存量(包括子函数占用的内存)
* cum%: 对应函数占用进程总申请内存的比例(包括子函数占用的内存)

### 3.3 cpu 采样分析
```
go tool pprof http://127.0.0.1:9002/debug/pprof/profile?seconds=30
```
执行 top 命令后，内容如下：
```
Fetching profile over HTTP from http://127.0.0.1:9002/debug/pprof/profile?seconds=30
Saved profile in /Users/huangjie5/pprof/pprof.samples.cpu.005.pb.gz
Type: cpu
Time: Oct 28, 2020 at 7:23pm (CST)
Duration: 30.14s, Total samples = 36.20s (120.12%)
Entering interactive mode (type "help" for commands, "o" for options)
(pprof) top
Showing nodes accounting for 35950ms, 99.31% of 36200ms total
Dropped 49 nodes (cum <= 181ms)
      flat  flat%   sum%        cum   cum%
   33720ms 93.15% 93.15%    35250ms 97.38%  main.fibo
    1530ms  4.23% 97.38%     1530ms  4.23%  runtime.newstack
     700ms  1.93% 99.31%      700ms  1.93%  runtime.mallocgc
         0     0% 99.31%    18010ms 49.75%  main.f2
         0     0% 99.31%    17940ms 49.56%  main.f3
         0     0% 99.31%      700ms  1.93%  runtime.makeslice
```
* flat: 对应函数占用 cpu 时间(不包括子函数占用的 cpu 时间)
* flat%: 对应函数占用 cpu 时间的比例(不包括子函数占用的 cpu 时间)
* sum%: 累计比例，从上到下累加
* cum: 对应函数占用 cpu 时间(包括子函数占用的 cpu 时间)
* cum%: 对应函数占用进程总申请内存的比例(包括子函数占用的 cpu 时间)

### 3.4 flat and cum
假设函数调用关系如下:
```
func f() {
    f1() // 第 1 步：耗时 1 秒
    f2() // 第 2 步：耗时 2 秒
    // do something // 第 3 步：耗时 3 秒
    f3() // 第 4 步：耗时 4 秒
}
```
* f() 函数的 flat 值为 3秒，即 第 3 步的时间，不包含子函数调用的时间
* f() 函数的 cum 值为 1+2+3+4=10 秒，包含所有子函数调用的时间

## 4 问题与解决思路
### 4.1 web 命令生成的文件不在浏览器中打开
问题：在使用 pprof 分析程序的时候，可以使用 web 方式查看函数调用关系，但是每次执行 web 命令的时候，都是用 sublime 或者其他代码编辑器打开文件，需要另存为文件之后再用浏览器打开，极不方便。
解决思路（mac）：
1. 在随意地方创建一个 test.svg 文件（touch ~/Downloads/test.svg）
2. 打开“仿达”，进入“下载目录”，找到“test.svg” 文件
3. 双手指点击“test.svg”，选择“显示简介”
4. 在简介界面，修改“打开方式”，选择任意浏览器，记得点击下方的“全部更改”，即刻生效。
写的有点啰嗦，其实就是在 mac 下修改文件的默认打开方式。

### 4.2 未装 graphviz
官网：http://www.graphviz.org
mac 安装方式：brew install graphviz
其他系统的安装方式自行 google

## 5 参考资料
1. [Profiling Go Programs](https://blog.golang.org/pprof)
2. [Diagnostics](https://tip.golang.org/doc/diagnostics.html)
3. [net/http/pprof](https://golang.org/pkg/net/http/pprof/)
4. [runtime/pprof](https://golang.org/pkg/runtime/pprof/)

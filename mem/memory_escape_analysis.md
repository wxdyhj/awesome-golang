# golang 内存逃逸分析

## 1 堆栈内存的区别
* 栈：编译器自动分配和释放，比如：函数的参数，局部变量等。函数运行结束，栈空间自动回收
* 堆：程序员自己申请和释放如果只申请不释放就会发生内存泄露，c 语言中使用 malloc 函数申请的内存就在堆上分配
* 很多 cpu 对压栈和出栈有硬件指令支持，即 PUSH 和 RELEASE（申请和释放），所以在栈上申请/释放内存速度极快；相比之下，在堆上申请/释放内存就很慢
* 堆内存和栈内存相比，堆内存适合不可预知大小的内存分配，或者申请教大块内存，但是堆内存分配速度较慢，也容易形成内存碎片。堆内存首先需要找寻找一块大小合适的内存块，且只能通过 gc 才能释放；而栈内存一般是在连续地址块上分配，进栈有序，退栈也有序，不会形成内存碎片
* 一般情况下，栈由高地址向低地址生长，堆从低地址向高地址生长
* 对于自带 gc 功能的 golang 编程语言，堆上内存需要触发 gc 才能回收，而栈上的内存作用域结束直接回收，无需 gc

所以，对于 gopher，为了使用更快速的栈内存，同时降低 gc 压力，对于函数内部的变量，如果函数运行结束后，不再使用，应尽量避免内存逃逸。但是如果函数运行结束后，外部仍需访问部分变量，那只能让变量逃逸到堆上。

## 2 什么是逃逸
逃逸指的是：本应该在栈上分配内存的变量，却在堆上分配。

逃逸未必是坏事，可通过逃逸的方式继续使用函数处理过的局部变量；当然也未必是好事，毕竟栈内存比堆内存的申请和释放快很多，同时过多的逃逸会给 gc 带来压力，也就会消耗更多的 cpu 资源。

代码中调用的函数运行在栈上，函数中声明的临时变量大部分会在栈上分配内存，函数运行结束后回收该段栈空间。不同函数的栈空间是相互独立的，其余代码不可访问。这句话容易理解，因为函数内部变量的作用域——函数内部的临时变量只在函数内部有效。

## 3 逃逸分析
### 3.1 c
【源码】
```
// grade.c
#include <stdio.h>

struct Grade {
    float Chinese;
    float Math;
    float English;
};

struct Grade* f1() {
    struct Grade grade = {90.0, 99.0, 98.5};
    return &grade;
}

struct Grade f2() {
    struct Grade grade = {90.0, 99.0, 98.5};
    return grade;
}

int main() {
    struct Grade* grade1 = f1();
    printf("Chinese: %.1f, Math: %.1f, English: %.1f\n", grade1->Chinese, grade1->Math, grade1->English);
    struct Grade grade2 = f2();
    printf("Chinese: %.1f, Math: %.1f, English: %.1f\n", grade2.Chinese, grade2.Math, grade2.English);
    return 0;
}
```

【编译环境】
```
[whoareyou@c]$ gcc --version
Configured with: --prefix=/Library/Developer/CommandLineTools/usr --with-gxx-include-dir=/Library/Developer/CommandLineTools/SDKs/MacOSX10.14.sdk/usr/include/c++/4.2.1
Apple LLVM version 10.0.1 (clang-1001.0.46.4)
Target: x86_64-apple-darwin18.7.0
Thread model: posix
InstalledDir: /Library/Developer/CommandLineTools/usr/bin
```

【编译结果】
```
[whoareyou@c]$ gcc grade.c
grade.c:11:13: warning: address of stack memory associated with local variable 'grade' returned
      [-Wreturn-stack-address]
    return &grade;
            ^~~~~
1 warning generated.
```

【运行结果】
```
[whoareyou@c]$ ./a.out
Chinese: 90.0, Math: 99.0, English: 98.5
Chinese: 90.0, Math: 99.0, English: 98.5
```

【分析】
* 根据编译过程输出的 warning 信息：```address of stack memory associated with local variable 'grade' returned```，中文大致翻译为：返回栈内存局部变量“grade”的地址
* c 语言不能返回局部变量的地址，虽然上述代码能正常输出，但迟早会 core dump
* f1() 函数返回变量本身，其实是经过一次内存拷贝，将栈内存中的变量拷贝到堆内存中

### 3.2 golang
【源码】
```
// main.go
package main

// WhoAreYou defines a struct to describe person
type WhoAreYou struct {
	Name string
}

// f1 returns the copy of the python
//go:noinline
func f1() WhoAreYou {
	python := WhoAreYou{Name: "python"}
	return python // 返回 python 变量本身，实际上发生一次栈内存到堆内存的拷贝，未发生逃逸
}

// f2 returns the address of the golang
//go:noinline
func f2() *WhoAreYou {
	golang := WhoAreYou{Name: "golang"}
	return &golang // 返回 golang 变量地址，导致 golang 变量发生逃逸
}

func main() {
	person1 := f1()
	person1copy := person1
	_ = person1copy
	person2 := f2()
	_ = person2
}
```

* ```//go:noinline``` 是个编译标记，让编译器不对指定函数进行内联，方便观察编译过程。

【编译环境】
```
[whoareyou@c]$ go version
go version go1.13.5 darwin/amd64
```

【编译结果】
```
[whoareyou@escape]$ go build -gcflags "-m -m"
# command-line-arguments
./main.go:11:6: cannot inline f1: marked go:noinline
./main.go:18:6: cannot inline f2: marked go:noinline
./main.go:23:6: cannot inline main: function too complex: cost 137 exceeds budget 80
./main.go:19:2: moved to heap: golang
```

* 编译时使用 gcflags 选项带上 -m 参数可查看编译状态，最多可以带 4 个 -m，带的越多输出的内容越详细
* 带有 4 个 -m 参数的命令为：```go build -gcflags "-m -m -m -m" main.go```

【分析】
* f1() 函数返回局部变量 python 本身，未发生逃逸，实际上存在一次栈内存拷贝
* person1copy := person1， 未发生逃逸，说明 person1copy 也是在栈上分配
* f2() 函数返回局部变量 golang 的地址，发生逃逸

## 4 常见内存逃逸分析
如果需要使用堆内存而发生逃逸，这没什么争议，但是编译器有时会将不需要使用堆内存的变量也逃逸到堆上，这时就可能出现性能问题。

多级间接赋值：对某个引用类型对象中的引用类成员进行赋值，go 语言中的引用数据类型有：func, interface, slice, map, chan, *Type(指针)。即 Data.Field = Value，如果 Data 和 Field 都是引用类数据类型，则会导致 Value 逃逸，注意：此处的 = 表示赋值或者参数传递。

### 4.1 slice and (interface or pointer)
* ```[]interface{}: data[0] = 100``` 的赋值语句会导致 100 逃逸
* ```[]*int: data[0] = &value``` 会使 value 逃逸
```
// f1.go
package main

//go:noinline
func f1() {
	data := make([]interface{}, 2, 2)
	data[0] = 110 // 110 会发生逃逸

	dataPointer := make([]*int, 2, 2)
	num := 119
	dataPointer[0] = &num // 会导致 num 发生逃逸

	dataInt := make([]int, 2, 2)
	dataInt[0] = 120 // 119 不会发生逃逸
	num2 := 122
	dataInt[1] = num2 // num2 不会发生逃逸
}

func main() {
	f1()
}
```
```
[whoareyou@examples]$ go build -gcflags "-m -m"
# whoareyou.com/learnxinyminutes/escape/examples
./f1.go:4:6: cannot inline f1: marked go:noinline
./f1.go:18:6: can inline main as: func() { f1() }
./f1.go:9:2: moved to heap: num
./f1.go:5:14: f1 make([]interface {}, 2, 2) does not escape
./f1.go:6:10: 110 escapes to heap
./f1.go:8:21: f1 make([]*int, 2, 2) does not escape
./f1.go:12:17: f1 make([]int, 2, 2) does not escape
```
显然，
* 110 发生逃逸，而 119 没有
* num 发生逃逸，而 num2 没有

### 4.2 map and (interface or slice or pointer)
* ```map[string]interface{}: data["key"] = value``` 会导致 value 逃逸（其中 value 为任意类型）
* ```map[interface{}]interface{}: data[key] = value``` 会导致 key 和 value 都逃逸
* ```map[string][]string: data["key"] = []string{"go"}``` 会导致切片逃逸
* map[string]*int: data["key"] = &value 会使 &value 逃逸
```
package main

//go:noinline
func f2() {
	m1 := make(map[string]interface{})
	m1["1"] = 110 // "1" 未发生逃逸，110 发生逃逸
	m2 := make(map[interface{}]interface{})
	m2["2"] = 119 // "2" 和 119 都发生逃逸
	m3 := make(map[string][]string)
	m3["3"] = []string{"go", "python"} // []string{"go", "python"} 发生逃逸
}

func main() {
	f2()
}
```

```
whoareyou@examples]$ go build -gcflags "-m -m" f2.go
# command-line-arguments
./f2.go:4:6: cannot inline f2: marked go:noinline
./f2.go:13:6: can inline main as: func() { f2() }
./f2.go:5:12: f2 make(map[string]interface {}) does not escape
./f2.go:6:10: 110 escapes to heap
./f2.go:7:12: f2 make(map[interface {}]interface {}) does not escape
./f2.go:8:5: "2" escapes to heap
./f2.go:8:10: 119 escapes to heap
./f2.go:9:12: f2 make(map[string][]string) does not escape
./f2.go:10:20: []string literal escapes to heap
```
显然，
* 110 发生逃逸
* "2" 和 119 都发生逃逸
* []string{"go", "python"} 发生逃逸

### 4.3 chan and (slice or interface or pointer)
* ```chan []int: data <- []int{110, 119}``` 会使 []string{110, 119} 逃逸

```
package main

//go:noinline
func f3() {
	chs := make(chan []int, 3)
	num := []int{110, 119}
	chs <- num // []int{110, 119} 发生逃逸

	chInterface := make(chan interface{}, 3)
	chInterface <- 120 // 120 逃逸

	chPointer := make(chan *int, 3)
	num1 := 122
	chPointer <- &num1 // num1 逃逸
}

func main() {
	f3()
}
```

```
[whoareyou@examples]$ go build -gcflags "-m -m" f3.go 
# command-line-arguments
./f3.go:4:6: cannot inline f3: marked go:noinline
./f3.go:17:6: can inline main as: func() { f3() }
./f3.go:13:2: moved to heap: num1
./f3.go:6:14: []int literal escapes to heap
./f3.go:10:14: 120 escapes to heap
```

显然，
* []int{110, 119} 发生逃逸
* 120 发生逃逸
* num1 发生逃逸

## 5 golang 内存逃逸实例
### 5.1 函数变量
如果变量值是一个函数，函数的参数又是引用类型，则传递给它的参数都会发生逃逸。

【源码】
```
package main

//go:noinline
func f1(num int) {}

//go:noinline
func f2(num *int) {}

func main() {
	m := 1
	n := 2
	p := 3
	q := 4
	f11 := f1 // 变量 f11 的值是一个函数
	f22 := f2 // 变量 f22 的值是一个函数

	// 直接调用
	f1(m)  // 不逃逸
	f2(&n) // 不逃逸

	// 间接调用
	f11(p)  // 不逃逸
	f22(&q) // 逃逸
}
```

【编译分析】
```
[whoareyou@escape]$ go build -gcflags "-m -m" main.go 
# command-line-arguments
./main.go:4:6: cannot inline f1: marked go:noinline
./main.go:7:6: cannot inline f2: marked go:noinline
./main.go:9:6: cannot inline main: function too complex: cost 272 exceeds budget 80
./main.go:7:9: f2 n does not escape
./main.go:13:2: moved to heap: q
```

【结果分析】
* f22 的类型是 func(n *int)，属于引用类型，函数参数 num 也是引用类型，所以调用 f22(&q) 会造成 q 发生逃逸，满足多级间接赋值的情况
* 余此类推，如果函数的参数类型是 slice，map 或者 interface{} 都会导致参数逃逸，不再重复举例

### 5.2 间接赋值
间接赋值指的是通过指向变量地址的指针进行赋值，也会造成逃逸。

【源码】
```
package main

type Data struct {
	m  map[int]int
	s  []int
	i  interface{}
	p  *int
	ch chan int
}

//go:noinline
func main() {
	num1 := 110
	d1 := Data{}              // d1 是变量本身，d1 本身不逃逸
	d1.m = make(map[int]int)  // make(map[int]int) 不逃逸
	d1.s = make([]int, 2)     // make([]int, 2) 不逃逸
	d1.i = 119                // 119 不逃逸
	d1.p = &num1              // num1 不逃逸
	d1.ch = make(chan int, 2) // make(chan int, 2) 不逃逸

	num2 := 120
	d2 := new(Data)           // d2 是指向 Data 的指针变量，d2 本身不逃逸
	d2.m = make(map[int]int)  // make(map[int]int) 逃逸
	d2.s = make([]int, 2)     // make([]int, 2) 逃逸
	d2.i = 122                // 122 逃逸
	d2.p = &num2              // 造成 num2 逃逸
	d2.ch = make(chan int, 2) // make(chan int, 2) 不逃逸
}
```

【编译分析】
```
[whoareyou@escape]$ go build -gcflags "-m -m" main.go 
# command-line-arguments
./main.go:12:6: cannot inline main: marked go:noinline
./main.go:21:2: moved to heap: num2
./main.go:15:13: main make(map[int]int) does not escape
./main.go:16:13: main make([]int, 2) does not escape
./main.go:17:7: main 119 does not escape
./main.go:22:11: main new(Data) does not escape
./main.go:23:13: make(map[int]int) escapes to heap
./main.go:24:13: make([]int, 2) escapes to heap
./main.go:25:7: 122 escapes to heap
```
【结果分析】
* d2 是指针，通过 d2 对引用类型成员进行赋值，会造成逃逸
* 如果成员是 channel 不会造成逃逸


### 5.3 接口类型
只要使用了 interface 类型（注意：不是 interface{}），那么赋值给它的变量一定会发生逃逸。因为 interfaceVariable.Method() 先是间接定位到它的实际值，在调用实际值的同名方法，执行时实际值作为参数传递给方法，相当于 interfaceVariable.Method.this = realValue。

【源码】
```
package main

type Animal interface {
	Say()
}

type Cat struct {
	Name string
}

//go:noinline
func (c *Cat) Say() {} // c 变量不逃逸

type Dog struct {
	Name string
}

//go:noinline
func (d Dog) Say() {} // d 变量不逃逸

//go:noinline
func main() {
	var animal1, animal2 Animal
	var cat1, cat2 *Cat
	cat1.Say()     // cat1 不逃逸
	animal1 = cat2 // 将 cat2 赋值给 animal1，并且调用 Say 方法，导致 cat2 逃逸
	animal1.Say()  // animal1 指向 Say 方法的指针逃逸

	var dog1, dog2 Dog
	dog1.Say()     // dog1 不逃逸
	animal2 = dog2 // 将 dog2 赋值给 animal2，并且调用 Say 方法，导致 dog2 逃逸
	animal2.Say()  // animal2 指向 Say 方法的指针不逃逸
}
```

【编译分析】
```
[whoareyou@escape]$ go build -gcflags "-m -m" main.go 
# command-line-arguments
./main.go:12:6: cannot inline (*Cat).Say: marked go:noinline
./main.go:19:6: cannot inline Dog.Say: marked go:noinline
./main.go:22:6: cannot inline main: marked go:noinline
./main.go:12:7: (*Cat).Say c does not escape
./main.go:19:7: Dog.Say d does not escape
./main.go:26:10: cat2 escapes to heap
./main.go:31:10: dog2 escapes to heap
<autogenerated>:1: leaking param: .this
<autogenerated>:1: (*Dog).Say .this does not escape
```

【结果分析】


### 5.4 引用类型的 channel
向 channel 中发送数据，本质上是为 channel 的内部成员赋值，所以 ```chan *Type, chan map[Type]Type, chan []Type, chan interface{}``` 类型都会导致发送到 channel 中的数据逃逸。不过这也是情理之中，因为 channel 就是为了实现不同协程之间能够共享数据，所以确实应该在堆上分配内存。

【源码】
```
package main

//go:noinline
func main() {
	chInt := make(chan *int, 2)
	chMap := make(chan map[int]int, 2)
	chSlice := make(chan []int, 2)
	chInterface := make(chan interface{}, 2)
	ch := make(chan int, 2)
	m := 110                 // m 变量发生逃逸
	n := map[int]int{10: 11} // map[int]int{10: 11} 发生逃逸
	p := []int{119}          // []int{119} 发生逃逸
	q := 120                 // 变量 q 发生逃逸
	x := 122                 // 变量 x 不逃逸

	chInt <- &m      // m 逃逸
	chMap <- n       // n 逃逸
	chSlice <- p     // p 逃逸
	chInterface <- q // q 逃逸
	ch <- x          // x 不逃逸
}
```

【编译分析】
```
[whoareyou@escape]$ go build -gcflags "-m -m" main.go 
# command-line-arguments
./main.go:4:6: cannot inline main: marked go:noinline
./main.go:10:2: moved to heap: m
./main.go:11:18: map[int]int literal escapes to heap
./main.go:12:12: []int literal escapes to heap
./main.go:19:14: q escapes to heap
```

【结果分析】
* 往 channel 发送变量地址，map, slice, interface 数据，造成这些数据发生逃逸
* 往 channel 发生 int 类型等变量，不会造成逃逸

### 5.5 可变参数
可变参数如 func(arg ...string) 实际与 func(arg []string) 是一样的，会增加一层访问路径。这也是 fmt.Println() 或 fmt.Sprintf 总是会使参数逃逸的原因。

【源码】
```
package main

import (
    "fmt"
)

//go:noinline
func main() {
	fmt.Println(110)
	s := fmt.Sprintf("%d", 119)
	fmt.Println(s)
}
```

【编译分析】
```
[whoareyou@escape]$ go build -gcflags "-m -m" main.go 
# command-line-arguments
./main.go:7:6: cannot inline main: function too complex: cost 218 exceeds budget 80
./main.go:8:13: inlining call to fmt.Println func(...interface {}) (int, error) { var fmt..autotmp_3 int; fmt..autotmp_3 = <N>; var fmt..autotmp_4 error; fmt..autotmp_4 = <N>; fmt..autotmp_3, fmt..autotmp_4 = fmt.Fprintln(io.Writer(os.Stdout), fmt.a...); return fmt..autotmp_3, fmt..autotmp_4 }
./main.go:10:13: inlining call to fmt.Println func(...interface {}) (int, error) { var fmt..autotmp_3 int; fmt..autotmp_3 = <N>; var fmt..autotmp_4 error; fmt..autotmp_4 = <N>; fmt..autotmp_3, fmt..autotmp_4 = fmt.Fprintln(io.Writer(os.Stdout), fmt.a...); return fmt..autotmp_3, fmt..autotmp_4 }
./main.go:8:14: 110 escapes to heap
./main.go:8:13: main []interface {} literal does not escape
./main.go:8:13: io.Writer(os.Stdout) escapes to heap
./main.go:9:18: main ... argument does not escape
./main.go:9:25: 119 escapes to heap
./main.go:10:13: s escapes to heap
./main.go:10:13: main []interface {} literal does not escape
./main.go:10:13: io.Writer(os.Stdout) escapes to heap
<autogenerated>:1: (*File).close .this does not escape
```
【结果分析】
* fmt.Println() 使 110 发生逃逸
* fmt.Sprintf() 使 119 发生逃逸
* fmt.Println() 使 变量 s 发生逃逸

## 6 总结
* 多级间接赋值会导致出现不必要的逃逸，这也是不推荐在 go 中使用指针的原因，因为它会增加一级访问路径
* 不要以为使用堆内存就会导致性能低下，使用栈内存就会带来性能优势。切记盲目优化，因为系统的性能瓶颈一般不会出现在内存分配上

## 7 参考资料
* [Golang Escape Analysis: reduce pressure on GC!](https://medium.com/faun/golang-escape-analysis-reduce-pressure-on-gc-6bde1891d625)
* [Golang escape analysis](http://www.agardner.me/golang/garbage/collection/gc/escape/analysis/2015/10/18/go-escape-analysis.html)
* [Golang内存逃逸是什么？怎么避免内存逃逸？](https://studygolang.com/articles/22875)
* [Go 语言内存管理（三）：逃逸分析](https://www.jianshu.com/p/518466b4ee96)
* [Go 逃逸分析的缺陷](https://studygolang.com/articles/12396?fr=sidebar)
* [Language Mechanics On Stacks And Pointers](https://www.ardanlabs.com/blog/2017/05/language-mechanics-on-stacks-and-pointers.html)
* [Language Mechanics On Escape Analysis](https://www.ardanlabs.com/blog/2017/05/language-mechanics-on-escape-analysis.html)

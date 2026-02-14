package main

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
)

// -----
// Methods with different receivers & signatures

func (p Point) Distance(q Point) float64 {
	return math.Hypot(p.X-q.X, p.Y-q.Y)
}

func (p *Point) Scale(factor float64) {
	p.X *= factor
	p.Y *= factor
}

func (e Employee) Name() string  { return "employee" }
func (e Employee) Age() int      { return 30 }
func (e Employee) Greet() string { return "Hello from " + e.Name() }
func (*Employee) String() string { return "<redacted>" }

func (e *Employee) GiveRaise(percent float64) {
	e.Salary *= (1 + percent/100)
}

// -----
// Generic function (Go 1.18+)

func functionWithParams(a int, b string) {
	fmt.Println(a, b)
}

type InterfaceType interface {
	Method()
}

func Max[T ~int | ~float64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// -----
// Function literals, closures, defer, panic, recover

func complicated() (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	defer fmt.Println("defer 1 - outermost")

	var once sync.Once
	once.Do(func() {
		fmt.Println("this runs only once")
	})

	closure := func(n int) func() int {
		count := n
		return func() int {
			count++
			return count
		}
	}

	inc := closure(100)
	_ = inc()
	_ = inc()

	panicIfNegative := func(v int) {
		if v < 0 {
			panic("negative value")
		}
	}

	panicIfNegative(5)
	// panicIfNegative(-1)   // commented — uncomment to test recover

	return 42, nil
}

// -----
// Every kind of operator we can reasonably use

func operatorsPlayground() {
	a, b := 10, 3

	_ = a + b
	_ = a - b
	_ = a * b
	_ = a / b
	_ = a % b
	_ = a & b
	_ = a | b
	_ = a ^ b
	_ = a &^ b
	_ = a << 2
	_ = a >> 1

	_ = a == b
	_ = a != b
	_ = a < b
	_ = a <= b
	_ = a > b
	_ = a >= b

	var x, y bool = true, false
	_ = x && y
	_ = x || y
	_ = !x

	f := 3.14
	_ = math.Floor(f) + 0.1

	s := "hello"
	t := "world"
	_ = s + t
	_ = s == t
	_ = s < t

	r := 'A'
	_ = unicode.IsLetter(r)
	_ = unicode.IsUpper(r)

	p := &a
	_ = *p

	ch := make(chan int)
	ch <- 1
	<-ch

	v, ok := <-ch
	_ = v
	_ = ok

	a++
	b--
}

// -----
// Control structures — almost everything

func controlStructures() {

	x := 10
	if x > 0 {
		_ = "positive"
	} else if x == 0 {
		_ = "zero"
	} else {
		_ = "negative"
	}

	if n := runtime.NumCPU(); n > 1 {
		fmt.Println("multi-core")
	}

	for i := 0; i < 3; i++ {
	}

	for range []int{1, 2, 3} {
	}

	for k, v := range map[string]int{"a": 1} {
		_ = k
		_ = v
	}

	switch os.Getenv("GOENV") {
	case "dev":
		fmt.Println("development")
	case "prod":
		fmt.Println("production")
	default:
		fmt.Println("unknown")
	}

	var i any = "hello"
	switch v := i.(type) {
	case string:
		_ = strings.ToUpper(v)
	case int:
		_ = v + 1
	case nil:
		// nothing
	default:
		_ = reflect.TypeOf(v)
	}

	ch1 := make(chan string)
	ch2 := make(chan int)
	select {
	case msg := <-ch1:
		_ = msg
	case ch2 <- 42:
	case <-time.After(100 * time.Millisecond):
	}

	go func() {
		_ = "goroutine"
	}()

	// labeled break / continue / goto (rare but valid)
loop:
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			if i*j == 6 {
				break loop
			}
			if i == 3 {
				continue loop
			}
		}
	}

	_ = "end"

	switch n := runtime.NumGoroutine(); n {
	case 1:
	default:
	}

	switch n := 1; v := i.(type) {
	case string:
		_ = n + len(v)
	default:
	}

	select {
	default:
	}

	for {
		break
	}

	for i := 0; i < 5; i++ {
		break
	}

	goto L
L:
}

// -----
// Literals of many kinds

func literals() {
	_ = 42
	_ = 0x2a
	_ = 0o52
	_ = 0b101010
	_ = 4_200_000_000

	_ = 3.14
	_ = 1e-10
	_ = 6.67430e-11

	_ = 2 + 3i
	_ = complex(1, 2)

	_ = 'π'
	_ = '\x41'
	_ = '\u2603'
	_ = '\U0001F30D'

	_ = "double quoted\n\t"
	_ = `raw string
		can have "quotes"
		and newlines`

	_ = []int{1, 2, 3}
	_ = [...]int{1, 2, 3}
	_ = [4]int{1, 2}
	_ = [...]int{9: 42}

	_ = map[string]int{"a": 1, "b": 2}
	_ = struct{ X int }{42}

	_ = make([]int, 10, 20)
	_ = new(int)

	_ = [...]byte{1, 2, 3, 4}
	_ = [...]rune{'a', 'b', 'c'}

	type Pair[A, B any] struct {
		First  A
		Second B
	}

	_ = Pair[int, string]{42, "hello"}
}

// -----
// Main — put many things together

func main() {

	operatorsPlayground()
	controlStructures()
	literals()

	p1 := Point{3, 4}
	p2 := Point{0, 0}
	fmt.Printf("Distance: %.2f\n", p1.Distance(p2))

	p1.Scale(2)

	e := Employee{
		Person:   nil,
		ID:       1001,
		Salary:   75000,
		JoinedAt: time.Now(),
	}
	e.GiveRaise(10.5)

	_ = sum(1, 2, 3)

	_, _ = complicated()

	_ = Max(3.14, 2.718)

	s := []int{0, 1, 2, 3, 4, 5}
	_ = s[1:4]
	_ = s[:3]
	_ = s[2:]
	_ = s[:]
	_ = s[1:4:6]

	var anyVal any = "123"
	str, ok := anyVal.(string)
	_ = str
	_ = ok

	_ = int(3.999)
	_ = string(65)
	_ = []byte("hello")
	_ = fmt.Sprintf("%x", 255)

	fmt.Println("done")

	(func() {})()

	var counterd int = 0
	counterd++

	s := []int{0, 1, 2, 3, 4, 5}
	_ = s[1:4]
	_ = s[:3]
	_ = s[2:]
	_ = s[:]
	_ = s[1:4:6]

	_ = s[2]
	_ = s[len(s)-1]
	mm := map[int]string{1: "one"}
	_ = mm[1]
}

type Point struct {
	X float64 `json:"x"`
	Y float64
}

func sum(nums ...int) int {
	sum := 0
	for _, n := range nums {
		sum += n
	}
	return sum
}

var rc <-chan int = make(<-chan int)

//

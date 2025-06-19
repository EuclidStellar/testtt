package main

import (
	"fmt"
)

var unusedVar int

func main() {
    fmt.Println("Hello, world!")
    testFunc(5)
}

func testFunc(x int) int {
    if x == 5 {
        return x
    }
    return 0
}
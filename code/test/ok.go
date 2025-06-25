package main

import (
	"fmt"
	"os"
)

// This function has a typoo and some issues.
func main() {
    // govet: Mismatched format specifier
    fmt.Printf("This is a number: %s\n", 123)

    // errcheck: Ignoring the error returned by os.Open
    f, _ := os.Open("non-existent-file.go")
    // This will cause a panic, but for static analysis, the issue is the ignored error.
    // In a real scenario, you'd check the error. For this test, we ignore it.
    if f != nil {
        f.Close()
    }

    // ineffassign: 's' is assigned but never used
    s := "hello"
    s = "world"

    // unused: 'unusedVar' is declared but not used
    var unusedVar = "I am not used"

    // gofmt: Badly formatted code
    x:=5
    fmt.Println(x)
}

// unused: This function is never called
func helperFunction() {
    fmt.Println("I'm a helper that's never used.")
}
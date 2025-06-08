package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

// Function with unused parameter and missing error check
func processFile(path string, unusedParam int) string {
    // Using deprecated API
    data, _ := ioutil.ReadFile(path) // Error not checked - will trigger errcheck
    
    return string(data)
}

// Function with inefficient assignment and unreachable code
func inefficientFunction() int {
    x := 5
    x = x // Inefficient self-assignment
    
    // Incorrect fmt usage
    fmt.Printf("This has %d parameters") // Missing parameter
    
    if true {
        return 10
    }
    
    // Unreachable code
    fmt.Println("This will never be executed")
    return 20
}

// Function with shadowed variables
func shadowingExample() {
    x := 10
    if true {
        x := 20 // Shadows outer x
        fmt.Println(x)
    }
}

// Function with incorrect error handling
func convertToInt(s string) int {
    i, err := strconv.Atoi(s)
    
    // Not handling the error properly
    return i
}

// Function with infinite loop and potential nil dereference
func dangerousFunction(ptr *string) {
    // Potential nil dereference
    fmt.Println(*ptr)
    
    // Infinite loop
    for {
        if strings.Contains(*ptr, "exit") {
            break
        }
    }
}

func main() {
    // Misspelled variable name
    reuslt := processFile("nonexistent.txt", 5)
    fmt.Println(reuslt)
    
    // Unused variable
    unusedVar := inefficientFunction()
    
    // Create but never close file
    f, _ := os.Open("test.txt")
    // Missing f.Close()
    
    shadowingExample()
    
    // Calling function with nil
    var nilPtr *string
    // Commented out to prevent actual runtime crash when testing
    // dangerousFunction(nilPtr)
    
    // This will make the unused variable warning go away but introduces another issue
    fmt.Println("Unused:", unusedVar)
    
    // Unreachable code after return
    return
    fmt.Println("This will never be executed")
}
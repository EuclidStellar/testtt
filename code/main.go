package main

import (
	"fmt"
	"os"
	"strconv"
)

// Function with unused variable and missing error check
func processFile(path string) string {
    unusedVar := "this will trigger unused warning"
    
    data, _ := os.ReadFile(path) // Error not checked - will trigger errcheck
    
    return string(data)
}

// Function with inefficient assignment
func inefficientFunction() {
    x := 5
    x = x // Inefficient self-assignment
    
    // Incorrect fmt usage
    fmt.Printf("This has %d parameters") // Missing parameter
}

// Function with incorrect error handling
func convertToInt(s string) int {
    i, err := strconv.Atoi(s)
    
    // Not handling the error properly
    return i
}

func main() {
    // Misspelled variable name
    reuslt := processFile("nonexistent.txt")
    fmt.Println(reuslt)
    
    // Unreachable code after return
    return
    fmt.Println("This will never be executed")
}
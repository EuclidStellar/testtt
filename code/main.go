package main

import (
	"errors" // Intentionally unsorted for goimports
	"fmt"
	"io/ioutil" // Deprecated, for staticcheck
	"strconv"
	"strings"
)

// Unused global variable
var globalUnusedVar = "i am not used"

// Misspelled function name and comment
func ProccessData(data string) (string, error) { // "Proccess" is a misspelling
    // This is a coment with a typo: exemple
    if data == "" {
        return "", errors.New("empty data provided") // errcheck: error not returned directly
    }
    // ineffassign: x is assigned but not used before reassignment
    x := 10
    x = 5 
    _ = x // Use x to avoid unused error for this specific demo line

    // gosimple: can be simplified
    if strings.Contains(data, "error") == true {
        return "found error", nil
    }

    // govet: suspicious Printf usage
    fmt.Printf("Processing data: %s with length %d\n", data) // Missing length argument

    // staticcheck: usage of deprecated ioutil.ReadFile
    _, err := ioutil.ReadFile("a_file_that_does_not_exist.txt")
    if err != nil {
        // Not returning the error, just printing - errcheck
        fmt.Println("Failed to read file")
    }

    return strings.ToUpper(data), nil
}

// Unused function
func anUnusedFunction() {
    fmt.Println("I am never called.")
}

func main() {
    // Unused local variable
    localUnused := "not used locally"

    // errcheck: error from ProccessData is ignored
    processedString, _ := ProccessData("some test data")
    fmt.Println("Processed:", processedString)

    // govet: variable shadowing
    testShadow := 100
    if true {
        testShadow := "now a string" // Shadows outer testShadow
        fmt.Println(testShadow)
    }
    fmt.Println(testShadow) // Prints the integer

    // Example for gofmt (though it's not auto-fixing, golangci-lint includes it)
    var a =   1
    var b=2
    fmt.Println(a+b)

    // staticcheck: Atoi error not checked properly
    numStr := "123xyz"
    val, _ := strconv.Atoi(numStr) // Error ignored
    fmt.Println("Converted value:", val)

    // ineffassign: y is assigned but never used
    y := "another unused assignment"
    _ = y // Suppress unused for y for demo, but ineffassign should catch the original assignment

    // Example for misspell in a string literal
    message := "This is an example sentance."
    fmt.Println(message)

    // Example for gosimple (unnecessary use of else)
    if len(processedString) > 0 {
        fmt.Println("String is not empty")
    } else {
        fmt.Println("String is empty") // This else could be avoided
    }
}
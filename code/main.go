package main

// goimports will reorder these and add a blank line between std and other libs.
import (
	"fmt"
	"io/ioutil" // staticcheck will suggest replacing this with 'os'.
	"os"
	"strings"
)

// misspell will correct this coment.
// This is an example coment with a typo.

func main() {
    // gofmt will fix this inconsistent formatting.
    var a   =  1
    var b=2
    fmt.Println(a + b)

    // gosimple (part of staticcheck) will simplify this condition.
    shouldPrint := true
    if shouldPrint == true {
        fmt.Println("Printing is enabled.")
    }

    // staticcheck will suggest a fix for using a deprecated function.
    // The linter can replace ioutil.ReadFile with os.ReadFile.
    data, err := ioutil.ReadFile("test.txt")
    if err != nil {
        // Another misspell target in a string literal.
        fmt.Fprintf(os.Stderr, "An imediate error occured: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("File content:", string(data))

    message := "All systems are funtional."
    fmt.Println(strings.ToUpper(message))
}

package main

// For goimports: imports are intentionally out of order.
// For staticcheck: ioutil is deprecated.
import (
    "fmt"
    "os"
    "errors"
    "io/ioutil" // Linter: staticcheck (deprecated)
    "net/http" // Linter: goimports (will be removed as it's unused)
)

// Linter: unused
// This function is never called.
func anUnusedFunction() {
    fmt.Println("This function is never called.")
}

// Linter: misspell
// This is a coment with a typo.
func main() {
    // Linter: gofmt
    // Inconsistent formatting.
    var a   =  1
    var b=2
    fmt.Println(a + b)

    // Linter: ineffassign
    // 'x' is assigned 10 but this value is never used before the next assignment.
    var x int
    x = 10
    x = 5
    _ = x // Use x to satisfy 'unused' for this demo line.

    // Linter: gosimple (part of staticcheck)
    // This condition can be simplified.
    isEnabled := true
    if isEnabled == true {
        fmt.Println("It's enabled!")
    }

    // Linter: govet
    // Incorrect number of arguments for format string.
    fmt.Printf("The value is: %d and %s\n", a)

    // Linter: errcheck
    // The error returned by errors.New is not checked.
    _ = errors.New("this is a new error")

    // Linter: staticcheck (deprecated package)
    // ioutil.ReadFile is deprecated and should be replaced with os.ReadFile.
    content, err := ioutil.ReadFile("a_file_that_doesnt_exist.txt")
    if err != nil {
        // Linter: misspell
        // Typo in a string literal.
        fmt.Fprintf(os.Stderr, "An error occured: %v\n", err)
    } else {
        fmt.Println(string(content))
    }

    // Linter: unused
    // This variable is declared but never used.
    var unusedLocalVariable = "I'm not used."
}
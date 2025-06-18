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
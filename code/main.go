package main

// For goimports: imports are intentionally out of order and one is unused
import (
	"errors"
	"fmt"
	"io/ioutil" // staticcheck: deprecated package
	"net/http"  // Example of an import that might be added by goimports if used
	"os"        // Unused import if not used below (but it will be)
	"strconv"
	"strings"
)

// unused: globalUnusedVar is an unused global variable
var globalUnusedVar = "I am globally unused"

// misspell: "Proces" instead of "Process", "coment" instead of "comment"
// govet: possible error in format string for Printf
// errcheck: error returned by errors.New is not checked by caller in one path
// staticcheck: SA5011: possible nil pointer dereference (if data is nil, though less likely with string)
func ProcesAndFormatData(data string, verbose bool) (string, error) {
	// This is a coment with a typo: exemple
	if data == "" {
		return "", errors.New("empty data input") // errcheck: if caller ignores error
	}

	// ineffassign: 'x' is assigned but never used before being reassigned
	var x int
	x = 10
	x = 5
	_ = x // Use x to satisfy 'unused' for this demo line, but ineffassign targets the first assignment

	// gosimple: can be simplified (e.g., 'if b == true' can be 'if b')
	if verbose == true {
		fmt.Println("Verbose mode enabled for data processing")
	}

	// govet: suspicious Printf usage (too few arguments for format string)
	fmt.Printf("Original data: %s, Length: %d\n", data)

	// staticcheck: S1000: should use strings.ToUpper instead of a loop (if a loop was here)
	// staticcheck: SA4006: this value of 'formattedData' is never used
	formattedData := strings.ToLower(data)
	formattedData = strings.ToUpper(data) // This assignment is used

	// errcheck: error from os.Stat is not checked
	fileInfo, _ := os.Stat("non_existent_file.txt")
	if fileInfo != nil {
		fmt.Println("File exists (unexpectedly for this demo)")
	}

	// gofmt: will fix spacing and alignment (example below)
	var a = 1
	var b = 2
	_ = a + b

	// staticcheck: U1000: unused variable (if not used later)
	anUnusedLocalVar := "locally unused"
	_ = anUnusedLocalVar // Suppress for demo, but staticcheck/unused would catch it

	// Using deprecated ioutil.ReadFile (staticcheck)
	_, readErr := ioutil.ReadFile("another_file.txt")
	if readErr != nil {
		fmt.Println("Read file error (not returning it - errcheck)")
	}

	// Example for goimports to add 'net/http' if we used http.MethodGet or similar
	_ = http.MethodGet // Using an element from net/http

	return formattedData, nil
}

// unused: anUnusedFunction is an unused function
func anUnusedFunction() {
	fmt.Println("This function is never called.")
}

func main() {
	// unused: 'result' is declared but its value is never used
	result, err := ProcesAndFormatData("  Test Data for Linters  ", true)
	if err != nil {
		// errcheck: error from ProcesAndFormatData is handled here
		fmt.Fprintf(os.Stderr, "Error processing data: %v\n", err)
		// return // Good practice to return or os.Exit on error
	}
	// fmt.Println("Final Result:", result) // If this line was commented, 'result' would be unused

	// govet: variable 'shadowVar' is shadowed in a nested block
	shadowVar := 100
	fmt.Println("Outer shadowVar:", shadowVar)
	if true {
		shadowVar := "I am a shadow" // This shadows the outer 'shadowVar'
		fmt.Println("Inner shadowVar:", shadowVar)
	}

	// staticcheck: Atoi error not checked
	// ineffassign: 'i' is assigned but never used
	i, _ := strconv.Atoi("123parseError")
	// fmt.Println("Parsed int:", i) // If this was commented, 'i' would be unused by staticcheck/unused

	// misspell: "Recieved" in string
	statusMessage := "Recieved and processed."
	fmt.Println(statusMessage)

	// gosimple: S1001: should use copy instead of a loop (if copying a slice element by element)
	// For this demo, let's show another simplifiable pattern:
	var flag bool
	if statusMessage != "" {
		flag = true
	} else {
		flag = false // This if/else can be simplified to flag = (statusMessage != "")
	}
	fmt.Println("Flag status:", flag)
}

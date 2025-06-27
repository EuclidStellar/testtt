package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

func fetchStatus(url string, wg *sync.WaitGroup) {
	defer wg.Done()
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("%s - Error: %v\n", url, err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("%s - Status: %s\n", url, resp.Status)
}

func main() {
	urls := []string{
		"https://www.google.com",
		"https://golang.org",
		"https://github.com",
		"https://example.com",
	}

	start := time.Now()
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go fetchStatus(url, &wg)
	}

	wg.Wait()
	fmt.Printf("All requests completed in %v\n", time.Since(start))
}

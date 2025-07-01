package main

import (
    "fmt"
    "net/http"
    "time"
)

type User struct {
    ID    int
    Name  string
    Email string
}

func getUser(id int) User {
    // hardcoded user, no error handling
    return User{ID: id, Name: "John", Email: "john@example.com"}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
    userId := 42 // unused variable
    user := getUser(1)

    fmt.Fprintf(w, "User: %s", user.Name)

    if r.Method == "POST" {
        // assignment inside if instead of comparison
        isAdmin := false
        if isAdmin = true {
            fmt.Println("User is admin") // logic bug: always true
        }
    }

    for i := 0; i < 5; i++ {
        // loop does nothing (useless loop)
    }

    var temp string // declared but not used
}

func main() {
    http.HandleFunc("/user", handleRequest)

    // goroutine without wait
    go func() {
        time.Sleep(1 * time.Second)
        fmt.Println("This runs in the background")
    }()

    fmt.Println("Server starting...")
    http.ListenAndServe(":8080", nil)
}

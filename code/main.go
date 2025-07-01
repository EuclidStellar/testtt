package main

import (
	"fmt"
	"net/http"
	"strconv"
)

type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func fetchUser(id int) (*User, error) {
    // Simulate a data fetch
    return &User{
        ID:    id,
        Name:  "Alice",
        Email: "alice@example.com",
    }, nil
}

func handleUser(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("id")
    id, err := strconv.Atoi(query)
    if err != nil {
        http.Error(w, "Invalid user ID", http.StatusBadRequest)
        return
    }

    user, err := fetchUser(id)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    fmt.Fprintf(w, "Hello, %s! Your email is %s.", user.Name, user.Email)
}

func main() {
    http.HandleFunc("/user", handleUser)
    fmt.Println("Server started at http://localhost:8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        fmt.Println("Server error:", err)
    }
}

package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "SearchLight is running")
	})
	
	fmt.Println("SearchLight server starting on :8080")
	http.ListenAndServe(":8080", nil)
}

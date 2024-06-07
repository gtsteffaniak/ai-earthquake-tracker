package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func setupWeb() {
	// Serve static files from the ./static directory
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/items", getItemsHandler)
	fmt.Println("Server listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getItemsHandler(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(tableData)
	if err != nil {
		log.Printf("Failed to marshal item: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// handle index as static info from ./static/index.html

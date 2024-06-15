package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

func setupWeb() {
	// Serve embedded files at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(r.URL.Path, "/")
		if filePath == "" {
			filePath = "static/index.html"
		} else {
			filePath = "static/" + filePath
		}
		data, err := staticFiles.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	})
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

//go:build e2ehelper

package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"ok":                  "true",
			"host":                r.Host,
			"x-real-ip":           r.Header.Get("X-Real-IP"),
			"x-forwarded-for":     r.Header.Get("X-Forwarded-For"),
			"x-forwarded-proto":   r.Header.Get("X-Forwarded-Proto"),
			"resource-identifier": "e2e-machine",
		})
	})
	log.Fatal(http.ListenAndServe(":32400", nil))
}

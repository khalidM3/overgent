// Command fakebackend stands in for convex-local-backend in the localbackend
// unit tests. It honors the same flags, answers the same three health and
// deploy2 routes, and can be told to fail on demand, so supervision, health
// timeouts, and bundle gating are testable without a 160 MB binary or a real
// Convex deployment.
//
// It is built as "convex-local-backend" by the test so the stale-process check,
// which matches on the command name, sees what it would see in production.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "keygen" {
		var instance, secret string
		for index, argument := range os.Args {
			switch argument {
			case "--instance-name":
				instance = os.Args[index+1]
			case "--instance-secret":
				secret = os.Args[index+1]
			}
		}
		fmt.Printf("%s|fake%s\n", instance, secret[:8])
		return
	}

	flags := map[string]string{}
	for index := 0; index < len(os.Args)-1; index++ {
		if strings.HasPrefix(os.Args[index], "--") {
			flags[os.Args[index]] = os.Args[index+1]
		}
	}
	// FAKE_BACKEND_MODE drives the failure the test is exercising.
	switch os.Getenv("FAKE_BACKEND_MODE") {
	case "exit":
		os.Exit(3)
	case "silent":
		// Bind nothing: the health budget has to expire.
		select {}
	}
	instance := flags["--instance-name"]
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unknown"))
	})
	mux.HandleFunc("/instance_name", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(instance))
	})
	mux.HandleFunc("/api/deploy2/start_push", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) || !brotliEncoded(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schemaChange": map[string]any{"id": "sch_fake"}})
	})
	mux.HandleFunc("/api/deploy2/wait_for_schema", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		state := "complete"
		if os.Getenv("FAKE_BACKEND_MODE") == "incompatible" {
			state = "failed"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": state})
	})
	mux.HandleFunc("/api/deploy2/finish_push", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) || !brotliEncoded(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"componentDiffs": map[string]any{}})
	})
	mux.HandleFunc("/api/update_environment_variables", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	address := flags["--interface"] + ":" + flags["--port"]
	if err := http.ListenAndServe(address, mux); err != nil {
		os.Exit(4)
	}
}

// authorized asserts the two headers the pinned replay must send. Getting these
// wrong is the failure the pin exists to catch, so the fake refuses them rather
// than accepting anything.
func authorized(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Convex ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if !strings.HasPrefix(r.Header.Get("Convex-Client"), "npm-cli-") {
		http.Error(w, "unsupported client", http.StatusBadRequest)
		return false
	}
	return true
}

// brotliEncoded asserts the compression the pinned CLI applies to the two large
// deploy2 bodies. The replay sending them uncompressed is a silent difference
// from what the endpoint was recorded against.
func brotliEncoded(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Content-Encoding") != "br" {
		http.Error(w, "expected brotli", http.StatusBadRequest)
		return false
	}
	return true
}

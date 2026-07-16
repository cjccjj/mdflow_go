//go:build !diagnose

package main

import "fmt"

func main() {
	fmt.Println("mdflow-diagnose is an internal diagnostic; run with: go run -tags diagnose ./cmd/mdflow-diagnose")
}

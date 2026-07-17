// Package commonmark contains test and diagnostic support for comparing
// mdflow's streaming parser with the CommonMark specification. It is kept in
// internal so it cannot become part of the renderer's public API.
package commonmark

import (
	"encoding/json"
	"os"
)

const (
	// DefaultSpecFile is relative to the repository root. Callers that run
	// from another directory should pass an explicit path to Load.
	DefaultSpecFile = "dev_docs/spec.json"
)

// Example is one CommonMark specification fixture.
type Example struct {
	Number   int    `json:"example"`
	Section  string `json:"section"`
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Line     int    `json:"start_line"`
}

// jsonExample is the raw JSON structure for decoding.
type jsonExample struct {
	Markdown  string `json:"markdown"`
	HTML      string `json:"html"`
	Example   int    `json:"example"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Section   string `json:"section"`
}

// Load reads the pre-extracted CommonMark examples from spec.json.
func Load(path string) ([]Example, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw []jsonExample
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, err
	}

	examples := make([]Example, len(raw))
	for i, r := range raw {
		examples[i] = Example{
			Number:   r.Example,
			Section:  r.Section,
			Markdown: r.Markdown,
			HTML:     r.HTML,
			Line:     r.StartLine,
		}
	}
	return examples, nil
}

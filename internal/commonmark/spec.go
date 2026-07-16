// Package commonmark contains test and diagnostic support for comparing
// mdflow's streaming parser with the CommonMark specification. It is kept in
// internal so it cannot become part of the renderer's public API.
package commonmark

import (
	"bufio"
	"os"
	"strings"
)

const (
	// DefaultSpecFile is relative to the repository root. Callers that run
	// from another directory should pass an explicit path to Load.
	DefaultSpecFile = "dev_docs/commonMark_spec.txt"

	exampleFence = "```````````````````````````````` example"
	exampleClose = "````````````````````````````````"
)

// Example is one CommonMark specification fixture.
type Example struct {
	Number   int
	Section  string
	Markdown string
	HTML     string
	Line     int
}

// Load reads CommonMark's source specification directly and extracts its
// embedded examples. The source file remains the sole fixture authority.
func Load(path string) ([]Example, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var examples []Example
	var currentSection string
	var inExample bool
	var beforeDot bool
	var markdown strings.Builder
	var expectedHTML strings.Builder
	var number int
	var exampleLine int

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if !inExample {
			if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
				heading := strings.TrimSpace(strings.TrimLeft(line, "# "))
				if heading != "" {
					currentSection = heading
				}
			}
			if line == exampleFence {
				inExample = true
				beforeDot = true
				markdown.Reset()
				expectedHTML.Reset()
				number++
				exampleLine = lineNumber
			}
			continue
		}

		if line == "." && beforeDot {
			beforeDot = false
			continue
		}

		if line == exampleClose {
			examples = append(examples, Example{
				Number:   number,
				Section:  currentSection,
				Markdown: strings.ReplaceAll(markdown.String(), "→", "\t"),
				HTML:     strings.ReplaceAll(expectedHTML.String(), "→", "\t"),
				Line:     exampleLine,
			})
			inExample = false
			continue
		}

		if beforeDot {
			markdown.WriteString(line)
			markdown.WriteByte('\n')
		} else {
			expectedHTML.WriteString(line)
			expectedHTML.WriteByte('\n')
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return examples, nil
}

//go:build diagnose

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cjccjj/mdflow/internal/commonmark"
	"github.com/cjccjj/mdflow/pkg/markdown"
	"github.com/cjccjj/mdflow/pkg/markdown/event"
	"github.com/cjccjj/mdflow/pkg/markdown/parser"
	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

const inventoryVersion = 1

type options struct {
	specPath       string
	example        int
	input          string
	inputFile      string
	expectedHTML   string
	expectedFile   string
	chunks         string
	jsonOutput     bool
	writeInventory string
	baseline       string
	minimize       bool
	limit          int
	tradeoffs      string
}

type inventory struct {
	Version   int          `json:"version"`
	ChunkPlan string       `json:"chunk_plan"`
	Cases     []caseRecord `json:"cases"`
}

type caseRecord struct {
	Number         int    `json:"number"`
	Section        string `json:"section,omitempty"`
	Status         string `json:"status"`
	Classification string `json:"classification"`
	Reason         string `json:"reason,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Difference     string `json:"difference,omitempty"`
}

type caseReport struct {
	caseRecord
	Input              string                   `json:"input,omitempty"`
	ExpectedHTML       string                   `json:"expected_html,omitempty"`
	ExpectedOperations []commonmark.Operation   `json:"expected_operations,omitempty"`
	ActualOperations   []commonmark.Operation   `json:"actual_operations,omitempty"`
	Trace              []parser.TraceTransition `json:"trace,omitempty"`
	RenderedOutput     string                   `json:"rendered_output,omitempty"`
	MinimizedInput     string                   `json:"minimized_input,omitempty"`
}

type cluster struct {
	Signature string `json:"signature"`
	Count     int    `json:"count"`
	First     int    `json:"first_example"`
	Examples  []int  `json:"examples"`
}

func main() {
	var opts options
	flag.StringVar(&opts.specPath, "spec", commonmark.DefaultSpecFile, "path to commonMark_spec.txt")
	flag.IntVar(&opts.example, "example", 0, "CommonMark example number")
	flag.StringVar(&opts.input, "input", "", "Markdown input to diagnose")
	flag.StringVar(&opts.inputFile, "input-file", "", "file containing Markdown input")
	flag.StringVar(&opts.expectedHTML, "expected-html", "", "expected HTML for -input semantic comparison")
	flag.StringVar(&opts.expectedFile, "expected-file", "", "file containing expected HTML")
	flag.StringVar(&opts.chunks, "chunks", "line", "chunk plan: none, line, rune, or comma-separated byte positions")
	flag.BoolVar(&opts.jsonOutput, "json", false, "emit machine-readable JSON")
	flag.StringVar(&opts.writeInventory, "write-inventory", "", "write compact full-spec inventory JSON")
	flag.StringVar(&opts.baseline, "baseline", "", "compare results with an existing inventory JSON")
	flag.BoolVar(&opts.minimize, "minimize", false, "minimize a selected semantic mismatch")
	flag.IntVar(&opts.limit, "limit", 0, "limit full-spec printable results (0 means all summaries)")
	flag.StringVar(&opts.tradeoffs, "tradeoffs", "dev_docs/Streaming_Limitations.md", "documented streaming tradeoffs file")
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "mdflow-diagnose:", err)
		os.Exit(2)
	}
}

func run(opts options) error {
	if opts.inputFile != "" {
		data, err := os.ReadFile(opts.inputFile)
		if err != nil {
			return err
		}
		opts.input = string(data)
	}
	if opts.expectedFile != "" {
		data, err := os.ReadFile(opts.expectedFile)
		if err != nil {
			return err
		}
		opts.expectedHTML = string(data)
	}

	if opts.input != "" || opts.inputFile != "" {
		hasOracle := opts.expectedHTML != "" || opts.expectedFile != ""
		report, err := diagnose(caseRecord{}, opts.input, opts.expectedHTML, opts.chunks, hasOracle)
		if err != nil {
			return err
		}
		if opts.minimize && report.Status == "mismatch" {
			report.MinimizedInput = minimize(opts.input, opts.expectedHTML, opts.chunks, report.Signature)
		}
		return writeSingleReport(report, opts.jsonOutput)
	}

	examples, err := commonmark.Load(opts.specPath)
	if err != nil {
		return err
	}
	if len(examples) == 0 {
		return errors.New("no CommonMark examples found")
	}

	if opts.example != 0 {
		for _, example := range examples {
			if example.Number != opts.example {
				continue
			}
			report, err := diagnose(caseRecord{Number: example.Number, Section: example.Section}, example.Markdown, example.HTML, opts.chunks, true)
			if err != nil {
				return err
			}
			if opts.minimize && report.Status == "mismatch" {
				report.MinimizedInput = minimize(example.Markdown, example.HTML, opts.chunks, report.Signature)
			}
			return writeSingleReport(report, opts.jsonOutput)
		}
		return fmt.Errorf("example %d was not found", opts.example)
	}

	reports := make([]caseReport, 0, len(examples))
	for _, example := range examples {
		report, err := diagnose(caseRecord{Number: example.Number, Section: example.Section}, example.Markdown, example.HTML, opts.chunks, true)
		if err != nil {
			return fmt.Errorf("example %d: %w", example.Number, err)
		}
		reports = append(reports, report)
	}

	inv := inventory{Version: inventoryVersion, ChunkPlan: opts.chunks, Cases: make([]caseRecord, len(reports))}
	for i, report := range reports {
		inv.Cases[i] = report.caseRecord
	}
	if opts.writeInventory != "" {
		if err := writeInventory(opts.writeInventory, inv); err != nil {
			return err
		}
	}

	if opts.jsonOutput {
		payload := struct {
			Inventory inventory `json:"inventory"`
			Clusters  []cluster `json:"clusters"`
		}{Inventory: inv, Clusters: clustersFor(inv.Cases)}
		return json.NewEncoder(os.Stdout).Encode(payload)
	}

	printSummary(inv, opts.limit)
	if opts.baseline != "" {
		baseline, err := readInventory(opts.baseline)
		if err != nil {
			return err
		}
		printDelta(baseline, inv, opts.tradeoffs)
	}
	return nil
}

func diagnose(record caseRecord, input, expectedHTML, chunkPlan string, hasOracle bool) (caseReport, error) {
	splits, err := splitPlan(input, chunkPlan)
	if err != nil {
		return caseReport{}, err
	}
	events, trace := parseWithTrace(input, splits)
	report := caseReport{caseRecord: record, Input: input, ExpectedHTML: expectedHTML}

	if !hasOracle {
		report.Status = "unoracled"
		report.Classification = "input_only"
		report.Trace = trace.Transitions
		return report, nil
	}

	expected := commonmark.ExpectedProjection(input, expectedHTML)
	actual := commonmark.ActualProjection(events)
	report.ExpectedOperations = expected.Operations
	report.ActualOperations = actual.Operations
	if !expected.Comparable {
		report.Status = "noncomparable"
		report.Classification = "unsupported_or_intentional"
		report.Reason = expected.Reason
		return report, nil
	}
	if !actual.Comparable {
		report.Status = "noncomparable"
		report.Classification = "unsupported_or_intentional"
		report.Reason = actual.Reason
		return report, nil
	}

	diff := commonmark.FirstDifference(expected.Operations, actual.Operations)
	if diff != nil {
		report.Status = "mismatch"
		report.Classification = "parser"
		report.Difference = differenceString(diff)
		cause := traceCauseIndex(trace.Transitions, diff)
		report.Trace = traceWindowAt(trace.Transitions, cause)
		report.Signature = signature(trace.Transitions, cause, diff)
		return report, nil
	}

	rendered, err := renderWithChunks(input, splits)
	if err != nil {
		report.Status = "renderer_error"
		report.Classification = "renderer"
		report.Reason = err.Error()
		return report, nil
	}
	report.Status = "match"
	report.Classification = "parser_and_renderer"
	report.RenderedOutput = rendered
	return report, nil
}

func parseWithTrace(input string, splits []int) ([]event.Event, *parser.Trace) {
	p := parser.New()
	trace := p.EnableTrace()
	var events []event.Event
	start := 0
	for _, split := range splits {
		if split <= start || split >= len(input) {
			continue
		}
		events = append(events, p.Parse(tokenizer.Tokenize([]byte(input[start:split])))...)
		start = split
	}
	if start < len(input) {
		events = append(events, p.Parse(tokenizer.Tokenize([]byte(input[start:])))...)
	}
	events = append(events, p.Flush()...)
	events = append(events, p.CloseStates()...)
	return events, trace
}

func renderWithChunks(input string, splits []int) (output string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("renderer panic: %v", recovered)
		}
	}()
	var buffer bytes.Buffer
	r := markdown.NewRenderer(&buffer)
	start := 0
	for _, split := range splits {
		if split <= start || split >= len(input) {
			continue
		}
		if _, err := r.Write([]byte(input[start:split])); err != nil {
			return "", err
		}
		start = split
	}
	if start < len(input) {
		if _, err := r.Write([]byte(input[start:])); err != nil {
			return "", err
		}
	}
	if err := r.Close(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func splitPlan(input, plan string) ([]int, error) {
	switch plan {
	case "", "line":
		return commonmark.LineBoundarySplits(input), nil
	case "none":
		return nil, nil
	case "rune":
		return commonmark.RuneBoundarySplits(input), nil
	}
	var splits []int
	for _, part := range strings.Split(plan, ",") {
		position, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || position <= 0 || position >= len(input) {
			return nil, fmt.Errorf("invalid chunk position %q", part)
		}
		if !utf8.ValidString(input[:position]) {
			return nil, fmt.Errorf("chunk position %d splits a UTF-8 rune", position)
		}
		splits = append(splits, position)
	}
	sort.Ints(splits)
	return compactInts(splits), nil
}

func compactInts(input []int) []int {
	output := input[:0]
	last := -1
	for _, value := range input {
		if value != last {
			output = append(output, value)
			last = value
		}
	}
	return output
}

func differenceString(diff *commonmark.Difference) string {
	expected := "<end>"
	actual := "<end>"
	if diff.Expected != nil {
		expected = diff.Expected.String()
	}
	if diff.Actual != nil {
		actual = diff.Actual.String()
	}
	return fmt.Sprintf("op[%d]: expected %s; actual %s", diff.Index, expected, actual)
}

func traceCauseIndex(transitions []parser.TraceTransition, diff *commonmark.Difference) int {
	if len(transitions) == 0 {
		return -1
	}
	match := len(transitions) - 1
	if diff.Actual != nil {
		for i, transition := range transitions {
			if transitionContains(transition, *diff.Actual) {
				match = i
				break
			}
		}
	}
	return match
}

func traceWindowAt(transitions []parser.TraceTransition, match int) []parser.TraceTransition {
	if len(transitions) == 0 || match < 0 {
		return nil
	}
	start := match - 3
	if start < 0 {
		start = 0
	}
	end := match + 3
	if end > len(transitions) {
		end = len(transitions)
	}
	return append([]parser.TraceTransition(nil), transitions[start:end]...)
}

func transitionContains(transition parser.TraceTransition, wanted commonmark.Operation) bool {
	for _, event := range transition.Events {
		got, ok := traceEventOperation(event)
		if !ok || got.Kind != wanted.Kind {
			continue
		}
		if wanted.Value != "" && got.Value != wanted.Value {
			continue
		}
		if wanted.Level != 0 && got.Level != wanted.Level {
			continue
		}
		return true
	}
	return false
}

func traceEventOperation(event parser.TraceEvent) (commonmark.Operation, bool) {
	switch event.Type {
	case "Text":
		return commonmark.Operation{Kind: "text", Value: event.Value}, true
	case "Newline":
		return commonmark.Operation{Kind: "newline"}, true
	case "HeaderStart":
		return commonmark.Operation{Kind: "heading_start", Level: event.Level}, true
	case "HeaderEnd":
		return commonmark.Operation{Kind: "heading_end"}, true
	case "BoldStart":
		return commonmark.Operation{Kind: "strong_start"}, true
	case "BoldEnd":
		return commonmark.Operation{Kind: "strong_end"}, true
	case "ItalicStart":
		return commonmark.Operation{Kind: "em_start"}, true
	case "ItalicEnd":
		return commonmark.Operation{Kind: "em_end"}, true
	case "InlineCodeStart":
		return commonmark.Operation{Kind: "code_start"}, true
	case "InlineCodeEnd":
		return commonmark.Operation{Kind: "code_end"}, true
	case "CodeBlockStart":
		return commonmark.Operation{Kind: "code_block_start"}, true
	case "CodeBlockEnd":
		return commonmark.Operation{Kind: "code_block_end"}, true
	case "CodeBlockLang":
		return commonmark.Operation{Kind: "code_lang", Value: event.Value}, true
	case "HorizontalRule":
		return commonmark.Operation{Kind: "thematic_break"}, true
	case "BulletItem":
		kind := "bullet"
		if event.Value != "" {
			kind = "ordered"
		}
		return commonmark.Operation{Kind: "list_item", Value: kind}, true
	case "BlockquoteStart":
		return commonmark.Operation{Kind: "blockquote_start"}, true
	case "BlockquoteEnd":
		return commonmark.Operation{Kind: "blockquote_end"}, true
	case "Link", "AutolinkURL", "AutolinkEmail":
		return commonmark.Operation{Kind: "link", Value: event.Value, URL: event.URL, Title: event.Title}, true
	case "Image":
		return commonmark.Operation{Kind: "image", Value: event.Value, URL: event.URL, Title: event.Title}, true
	default:
		return commonmark.Operation{}, false
	}
}

func signature(trace []parser.TraceTransition, cause int, diff *commonmark.Difference) string {
	branch, state, outcome := "trace.none", "unknown", "unknown"
	if cause >= 0 && cause < len(trace) {
		transition := trace[cause]
		branch, state, outcome = transition.Branch, transition.PreState, transition.Outcome
	}
	expected := "<end>"
	actual := "<end>"
	if diff.Expected != nil {
		expected = diff.Expected.String()
	}
	if diff.Actual != nil {
		actual = diff.Actual.String()
	}
	return strings.Join([]string{branch, state, expected, actual, outcome}, " | ")
}

func writeSingleReport(report caseReport, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	fmt.Printf("status: %s (%s)\n", report.Status, report.Classification)
	if report.Number != 0 {
		fmt.Printf("example: %d — %s\n", report.Number, report.Section)
	}
	if report.Reason != "" {
		fmt.Printf("reason: %s\n", report.Reason)
	}
	if report.Difference != "" {
		fmt.Printf("first divergence: %s\n", report.Difference)
		fmt.Printf("signature: %s\n", report.Signature)
	}
	if report.MinimizedInput != "" {
		fmt.Printf("minimized input:\n%s", report.MinimizedInput)
		if !strings.HasSuffix(report.MinimizedInput, "\n") {
			fmt.Println()
		}
	}
	if len(report.Trace) > 0 {
		fmt.Println("trace around first divergence:")
		for _, transition := range report.Trace {
			fmt.Printf("  #%d %s %s→%s token=%d:%s(%q) buffer=%d→%d events=%d outcome=%s\n",
				transition.Sequence, transition.Branch, transition.PreState, transition.PostState,
				transition.HeadTokenOrdinal, transition.HeadTokenType, transition.HeadTokenPreview,
				transition.BufferBefore, transition.BufferAfter, len(transition.Events), transition.Outcome)
		}
	}
	return nil
}

func writeInventory(path string, inv inventory) error {
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func readInventory(path string) (inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inventory{}, err
	}
	var inv inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return inventory{}, err
	}
	if inv.Version != inventoryVersion {
		return inventory{}, fmt.Errorf("unsupported inventory version %d", inv.Version)
	}
	return inv, nil
}

func clustersFor(cases []caseRecord) []cluster {
	bySignature := make(map[string]*cluster)
	for _, record := range cases {
		if record.Status != "mismatch" || record.Classification != "parser" || record.Signature == "" {
			continue
		}
		entry := bySignature[record.Signature]
		if entry == nil {
			entry = &cluster{Signature: record.Signature, First: record.Number}
			bySignature[record.Signature] = entry
		}
		entry.Count++
		entry.Examples = append(entry.Examples, record.Number)
		if record.Number < entry.First {
			entry.First = record.Number
		}
	}
	output := make([]cluster, 0, len(bySignature))
	for _, entry := range bySignature {
		output = append(output, *entry)
	}
	sort.Slice(output, func(i, j int) bool {
		if output[i].Count != output[j].Count {
			return output[i].Count > output[j].Count
		}
		if output[i].First != output[j].First {
			return output[i].First < output[j].First
		}
		return output[i].Signature < output[j].Signature
	})
	return output
}

func printSummary(inv inventory, limit int) {
	counts := make(map[string]int)
	for _, record := range inv.Cases {
		counts[record.Status]++
	}
	fmt.Printf("CommonMark inventory: %d cases; match=%d mismatch=%d noncomparable=%d renderer_error=%d\n",
		len(inv.Cases), counts["match"], counts["mismatch"], counts["noncomparable"], counts["renderer_error"])
	clusters := clustersFor(inv.Cases)
	fmt.Printf("parser mismatch clusters: %d\n", len(clusters))
	for _, entry := range clusters {
		fmt.Printf("  %d cases (first %d): %s\n", entry.Count, entry.First, entry.Signature)
	}
	if limit == 0 {
		return
	}
	shown := 0
	for _, record := range inv.Cases {
		if record.Status == "match" || record.Status == "noncomparable" {
			continue
		}
		fmt.Printf("  ex %d: %s — %s\n", record.Number, record.Status, record.Difference)
		shown++
		if shown == limit {
			break
		}
	}
}

func printDelta(before, after inventory, tradeoffsPath string) {
	beforeByNumber := make(map[int]caseRecord, len(before.Cases))
	for _, record := range before.Cases {
		beforeByNumber[record.Number] = record
	}
	var changed []int
	for _, record := range after.Cases {
		old, ok := beforeByNumber[record.Number]
		if !ok || old.Status != record.Status || old.Signature != record.Signature {
			changed = append(changed, record.Number)
		}
	}
	fmt.Printf("baseline delta: %d changed case(s)\n", len(changed))
	if len(changed) > 0 {
		fmt.Printf("changed examples: %s\n", joinInts(changed))
	}

	beforeClusters := make(map[string]bool)
	for _, entry := range clustersFor(before.Cases) {
		beforeClusters[entry.Signature] = true
	}
	var newClusters []cluster
	for _, entry := range clustersFor(after.Cases) {
		if !beforeClusters[entry.Signature] {
			newClusters = append(newClusters, entry)
		}
	}
	if len(newClusters) > 0 {
		fmt.Printf("new parser clusters: %d\n", len(newClusters))
		for _, entry := range newClusters {
			fmt.Printf("  %d cases (first %d): %s\n", entry.Count, entry.First, entry.Signature)
		}
	}
	if hits := tradeoffHits(tradeoffsPath, changed); len(hits) > 0 {
		fmt.Printf("changed documented streaming tradeoffs: %s\n", strings.Join(hits, "; "))
	}
}

func tradeoffHits(path string, changed []int) []string {
	data, err := os.ReadFile(path)
	if err != nil || len(changed) == 0 {
		return nil
	}
	changedSet := make(map[int]bool, len(changed))
	for _, number := range changed {
		changedSet[number] = true
	}
	var hits []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "Examples:") {
			continue
		}
		for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r < '0' || r > '9' }) {
			number, err := strconv.Atoi(field)
			if err == nil && changedSet[number] {
				hits = append(hits, strings.TrimSpace(line))
				break
			}
		}
	}
	return hits
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ", ")
}

func minimize(input, expectedHTML, chunkPlan, wantedSignature string) string {
	if strings.Contains(chunkPlan, ",") {
		// Explicit byte positions do not survive deletion. Keep this invariant
		// rather than silently testing a different chunk plan.
		return input
	}
	preserves := func(candidate string) bool {
		report, err := diagnose(caseRecord{}, candidate, expectedHTML, chunkPlan, true)
		return err == nil && report.Status == "mismatch" && report.Signature == wantedSignature
	}
	current := input
	for _, splitter := range []func(string) []string{splitLines, splitRunes, splitTokens} {
		units := splitter(current)
		if len(units) < 2 {
			continue
		}
		units = minimizeUnits(units, preserves)
		current = strings.Join(units, "")
	}
	return current
}

func minimizeUnits(units []string, preserves func(string) bool) []string {
	for width := len(units) / 2; width >= 1; width /= 2 {
		removed := false
		for start := 0; start+width <= len(units); start++ {
			candidate := append(append([]string(nil), units[:start]...), units[start+width:]...)
			if len(candidate) == 0 || !preserves(strings.Join(candidate, "")) {
				continue
			}
			units = candidate
			removed = true
			break
		}
		if removed {
			width = len(units)/2 + 1
		}
	}
	return units
}

func splitLines(input string) []string {
	if input == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := range input {
		if input[i] == '\n' {
			lines = append(lines, input[start:i+1])
			start = i + 1
		}
	}
	if start < len(input) {
		lines = append(lines, input[start:])
	}
	return lines
}

func splitRunes(input string) []string {
	units := make([]string, 0, utf8.RuneCountInString(input))
	for _, r := range input {
		units = append(units, string(r))
	}
	return units
}

func splitTokens(input string) []string {
	tokens := tokenizer.Tokenize([]byte(input))
	units := make([]string, len(tokens))
	for i, token := range tokens {
		units[i] = token.Value
	}
	return units
}

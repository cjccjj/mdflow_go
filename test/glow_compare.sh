#!/bin/bash

# Define the ANSI escape code for a full style reset
RESET=$'\e[0m'

# Optional section filter:
#   ./compare.sh Tabs
#   ./compare.sh "Fenced code blocks"
SECTION_FILTER="${1:-}"

# Force tools to output ANSI colors even when captured into a variable
export CLICOLOR_FORCE=1
export FORCE_COLOR=1

# Ensure jq is installed to extract the JSON blocks cleanly
if ! command -v jq &> /dev/null; then
    printf '%sError: %s is required to parse the JSON spec file.\n' "$RESET" "'jq'"
    exit 1
fi

JSON_FILE="spec.json"
GLOW_STYLE="./glow_clean.json"

if [ ! -f "$JSON_FILE" ]; then
    printf '%sError: Cannot find %s in the current directory.\n' "$RESET" "$JSON_FILE"
    exit 1
fi

if [ ! -f "$GLOW_STYLE" ]; then
    printf '%sError: Cannot find %s in the current directory.\n' "$RESET" "$GLOW_STYLE"
    exit 1
fi

if [ -n "$SECTION_FILTER" ]; then
    printf '%sFiltering section: %s\n' "$RESET" "$SECTION_FILTER"
fi

# Read each case json block cleanly using jq
jq -c --arg section "$SECTION_FILTER" '
    .[]
    | select($section == "" or .section == $section)
' "$JSON_FILE" | while read -r item; do

    # Extract metadata
    EXAMPLE=$(printf '%s\n' "$item" | jq -r '.example // "Unknown"')
    SECTION=$(printf '%s\n' "$item" | jq -r '.section // "Unknown"')

    # Capture mdflow output into a variable via strict piping
    MDFLOW_OUT=$(printf '%s\n' "$item" | jq -r '.markdown // ""' | go run /home/cj/projects/mdflow/cmd/mdflow/main.go)

    # Capture glow output using your custom clean JSON stylesheet
    GLOW_OUT=$(printf '%s\n' "$item" | jq -r '.markdown // ""' | PAGER=cat glow -s "$GLOW_STYLE")

    # Compare the exact string outputs, including ANSI escape codes
    if [ "$MDFLOW_OUT" = "$GLOW_OUT" ]; then
        printf '%sExample %s [%s] - SAME\n' "$RESET" "$EXAMPLE" "$SECTION"
    else
        printf '%sExample %s [%s] - DIFF  mdflow - glow\n' "$RESET" "$EXAMPLE" "$SECTION"
        printf '%s\n' "=============================="
        printf '%s\n' "$MDFLOW_OUT"
        printf '%s%s\n' "$RESET" "------------------------------"
        printf '%s\n' "$GLOW_OUT"
        printf '%s%s\n' "$RESET" "=============================="
    fi

    # Pause before continuing to the next example.
    # Use /dev/tty because the while loop stdin is coming from jq.
    printf '%sPress Enter to continue to the next example...' "$RESET" > /dev/tty
    IFS= read -r _ < /dev/tty
    printf '\n' > /dev/tty

done
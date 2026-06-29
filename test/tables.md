# Streaming Markdown Table Render Strategy Test

These tables are designed to test line-by-line streaming table rendering, especially:
- early width guesses
- late column expansion
- wrapping decisions
- redraw strategy
- alignment handling
- narrow vs wide column behavior
- escaped pipes
- mixed short and long content

---

## Test 1: Late Wide Cell Appears After Many Short Rows

| ID | Name | Note |
|---|---|---|
| 1 | A | x |
| 2 | Bo | ok |
| 3 | Cy | yes |
| 4 | Dee | no |
| 5 | Eve | done |
| 6 | Fox | wait |
| 7 | Gus | pass |
| 8 | Hal | fine |
| 9 | Ivy | go |
| 10 | Jay | This note arrives late and is much wider than every previous note cell. |

---

## Test 2: Early Wide Cell Then Later Short Rows

| Key | Description | Status |
|---|---|---|
| alpha | This first row is very long and may force a wide initial layout immediately. | active |
| b | tiny | ok |
| c | short | ok |
| d | mid | pending |
| e | x | done |
| f | small | fail |
| g | compact | ok |

---

## Test 3: Width Grows Gradually Row By Row

| Step | Value | Comment |
|---|---|---|
| 1 | a | x |
| 2 | ab | xx |
| 3 | abc | xxx |
| 4 | abcd | xxxx |
| 5 | abcde | xxxxx |
| 6 | abcdefghij | ten chars |
| 7 | abcdefghijklmnopqrst | twenty characters here |
| 8 | abcdefghijklmnopqrstuvwxyz | alphabet length value |
| 9 | abcdefghijklmnopqrstuvwxyz1234567890 | value keeps growing |
| 10 | abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJ | near fifty chars |

---

## Test 4: Different Column Becomes Wide Late

| A | B | C | D |
|---|---|---|---|
| x | short | short | short |
| y | short | short | short |
| z | short | short | short |
| q | short | This column suddenly becomes much longer than expected after short rows. | short |
| r | short | short | short |
| s | This different column becomes very wide even later than column C did before. | short | short |
| t | short | short | And now the final column has the widest late-arriving content of the table. |

---

## Test 5: Narrow Numeric Column With Late Long Text

| # | Count | Label |
|---:|---:|:---|
| 1 | 7 | A |
| 2 | 42 | B |
| 3 | 105 | C |
| 4 | 9000 | D |
| 5 | 123456789 | E |
| 6 | 2 | This label is unexpectedly long and should test wrapping behavior. |
| 7 | 3 | short again |

---

## Test 6: Alignment With Changing Widths

| Left | Center | Right |
|:---|:---:|---:|
| a | b | 1 |
| short | mid | 22 |
| longer left text | centered | 333 |
| x | this center cell is much longer than earlier center values | 4444 |
| tiny | ok | 55555555555555555555 |
| final left cell becomes very long near the end | z | 6 |

---

## Test 7: Many Columns, One Late Outlier

| C1 | C2 | C3 | C4 | C5 | C6 |
|---|---|---|---|---|---|
| a | b | c | d | e | f |
| aa | bb | cc | dd | ee | ff |
| 1 | 2 | 3 | 4 | 5 | 6 |
| red | blue | green | cyan | gray | tan |
| cat | dog | owl | fox | eel | ant |
| ok | ok | ok | ok | ok | This final column suddenly contains a very long value that may require redraw. |

---

## Test 8: Wrapping Stress With Sentences

| Short | Medium | Long Text |
|---|---|---|
| a | alpha beta | This sentence is long enough to wrap in many renderers. |
| b | small words here | Another row with enough text to check consistent wrapping. |
| c | compact | Short |
| d | medium content grows | This long text includes several words that should wrap cleanly without breaking the layout. |
| e | x | SupercalifragilisticexpialidociousIsAReallyLongUnbrokenWord |
| f | final medium value | Mix of short words and oneVeryLongTokenWithoutSpacesInside |

---

## Test 9: Empty Cells Mixed With Later Content

| Col A | Col B | Col C |
|---|---|---|
|  |  |  |
| x |  |  |
|  | y |  |
|  |  | z |
| short | short | short |
|  | This cell appears after empty cells and is significantly wider. |  |
| final |  | This final column also expands late with a longer cell value. |

---

## Test 10: Escaped Pipes And Markdown-Like Content

| Token | Meaning | Example |
|---|---|---|
| `*` | asterisk | `*not italic*` |
| `#` | hash | `# not a heading` |
| `>` | quote | `> not a quote block` |
| `-` | dash | `- not a list item` |
| `|` | pipe | escaped pipe: `left \| right` |
| `[]()` | link syntax | [short link](https://example.com) |
| `code` | inline code | `this inline code cell is much longer than earlier examples` |

---

## Test 11: Late Header-Like Width Challenge

| X | Y | Z |
|---|---|---|
| 1 | 2 | 3 |
| aa | bb | cc |
| red | green | blue |
| yes | no | maybe |
| small | medium | large |
| this-row-has-a-very-long-first-column-value | tiny | tiny |
| tiny | this-row-has-a-very-long-second-column-value | tiny |
| tiny | tiny | this-row-has-a-very-long-third-column-value |

---

## Test 12: Similar Rows Then Sudden Shape Change

| Time | Event | Detail |
|---|---|---|
| 09:00 | start | ok |
| 09:01 | tick | ok |
| 09:02 | tick | ok |
| 09:03 | tick | ok |
| 09:04 | tick | ok |
| 09:05 | tick | ok |
| 09:06 | tick | ok |
| 09:07 | warning | A very verbose diagnostic message appears here after many stable short rows. |
| 09:08 | tick | ok |
| 09:09 | recovered | ok |

---

## Test 13: Mixed Character Lengths Around Fifty Chars

| One Char | Ten Chars | Around Fifty Chars |
|---|---|---|
| a | 1234567890 | 12345678901234567890123456789012345678901234567890 |
| b | abcdefghij | abcdefghij abcdefghij abcdefghij abcdefghij abcdef |
| c | short text | This cell is close to fifty characters for testing. |
| d | medium val | Another near fifty character value in the same column. |
| e | x | short |
| f | finalvalue | A late medium-long value after a short row appears here. |

---

## Test 14: Competing Wide Columns

| Product | Region | Owner | Status |
|---|---|---|---|
| A | US | Jo | ok |
| B | EU | Li | ok |
| C | APAC | Mo | ok |
| Product name suddenly becomes much longer | US | Jo | ok |
| D | Region name suddenly becomes much longer than before | Li | ok |
| E | EU | Owner name suddenly becomes much longer | ok |
| F | APAC | Mo | Status message suddenly becomes much longer than expected |

---

## Test 15: Dense Table For Redraw Threshold Testing

| A | B | C | D |
|---|---|---|---|
| 1 | x | y | z |
| 2 | xx | yy | zz |
| 3 | xxx | yyy | zzz |
| 4 | xxxx | yyyy | zzzz |
| 5 | xxxxx | yyyyy | zzzzz |
| 6 | xxxxxx | yyyyyy | zzzzzz |
| 7 | xxxxxxx | yyyyyyy | zzzzzzz |
| 8 | xxxxxxxx | yyyyyyyy | zzzzzzzz |
| 9 | xxxxxxxxx | yyyyyyyyy | zzzzzzzzz |
| 10 | xxxxxxxxxx | yyyyyyyyyy | zzzzzzzzzz |
| 11 | this value is suddenly much longer than the pattern suggested | y | z |
| 12 | x | this value is suddenly much longer than the pattern suggested | z |
| 13 | x | y | this value is suddenly much longer than the pattern suggested |

---

## Test 16: Long Unbroken Tokens Versus Normal Text

| Type | Content | Note |
|---|---|---|
| word | hello | short |
| phrase | hello world again | normal |
| sentence | this is a sentence with spaces that can wrap | wraps nicely |
| token | abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ | hard to wrap |
| path | /users/example/projects/markdown/table/render/streaming/test/file.txt | slash-separated |
| url | https://example.com/a/very/long/path/that/might/not/wrap/well | URL wrapping |
| code | constValueNameThatKeepsGoingWithoutAnyNaturalBreakPoint | camelCase token |

---

## Test 17: Very Small First Rows, Then Full Paragraph-Like Cell

| ID | Text |
|---|---|
| 1 | a |
| 2 | b |
| 3 | c |
| 4 | d |
| 5 | e |
| 6 | This final row contains a much larger chunk of prose, intended to test whether the streaming renderer decides to wrap only this row, expand the whole column, or redraw the table from the beginning. |

---

## Test 18: Short After Wide After Short

| Phase | Message |
|---|---|
| start | ok |
| warmup | ok |
| stable | ok |
| spike | This message is very long and may force a layout expansion or wrapping decision. |
| cool | ok |
| stable again | ok |
| final | short |

---

## Test 19: Multiple Late Expansions In One Table

| ID | Alpha | Beta | Gamma |
|---|---|---|---|
| 1 | a | b | c |
| 2 | aa | bb | cc |
| 3 | aaa | bbb | ccc |
| 4 | Alpha column gets much longer right here | b | c |
| 5 | a | Beta column gets much longer a little later | c |
| 6 | a | b | Gamma column gets much longer at the very end |
| 7 | short | short | short |

---

## Test 20: Table Followed By Normal Markdown

| Name | Value | Comment |
|---|---|---|
| A | 1 | short |
| B | 22 | medium |
| C | 333 | This comment is longer and appears near the end. |

After the table, this paragraph should render as normal Markdown and not be absorbed into the table.
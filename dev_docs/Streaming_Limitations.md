# Streaming Compatibility Tradeoffs

mdflow renders incrementally and never builds a document AST. These are
deliberate, individually recorded compatibility differences. The diagnostic
command reads `Examples:` lines here when it reports baseline deltas.

## List continuation and nesting

Examples: 4, 259, 264, 313, 316

- Trigger: a flat list marker is followed by blank lines, indented continuation
  paragraphs, nested items, or a later block whose ownership depends on the
  active list container.
- Indistinguishable prefix: `- item\n\n` can continue as a list paragraph,
  terminate the list, or begin a nested block only after later input arrives.
- Retained-input policy: mdflow emits flat item events immediately and keeps no
  list-container stack or whole-item buffer.
- Wait condition and EOF fallback: there is no speculative list wait; later
  lines use ordinary paragraph/code recognition, including at EOF.
- Reason: a complete container stack would delay visible item output and add
  unbounded nested structure to the streaming parser.

## Reference-link resolution

Examples: 198, 201, 202

- Trigger: a reference link needs a definition that may appear later in the
  document or before a streaming boundary.
- Indistinguishable prefix: `[text][label]` is not resolvable until a matching
  `[label]: destination` is known.
- Retained-input policy: definitions and references are emitted as local
  events; no document-wide definition map is retained.
- Wait condition and EOF fallback: references render as local reference
  information when no immediate resolution exists, including at EOF.
- Reason: global resolution conflicts with immediate output and bounded memory.

## Bounded repeated strong delimiters

Examples: 391, 427, 467, 468, 470

- Trigger: a 4–6 character delimiter run such as `____foo____`, or an inner
  `__strong__` run inside an open strong span.
- Indistinguishable prefix: an opener run cannot be classified correctly until
  its matching flanking run, a newline, or EOF is seen.
- Retained-input policy: only the current line remains in the parser token
  buffer; no document-level delimiter history is retained.
- Wait condition and EOF fallback: the local resolver waits for a closer,
  newline, or EOF; ordinary emphasis fallback handles unresolved input.
- Reason: this bounded resolver fixes common repeated runs while preserving
  immediate behavior for ordinary emphasis and the existing depth cap.

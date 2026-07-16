# Refactor Plan: CommonMark-Compatible Streaming Transducer

## 1. Goal and constraints

### Goal

Improve CommonMark compatibility while preventing the current pattern where fixing one example breaks several others.

The refactor should make it possible to:

- identify the exact parser decision responsible for a failure;
- determine whether a mismatch is fixable with existing input or requires future information;
- understand the regression impact of each change;
- fix groups of related failures rather than individual examples.

### Constraints

This is deliberately **not a conventional Markdown parser**:

- No full-document AST.
- No conversion to an AST-based CommonMark architecture.
- No requirement for a clean grammar or complete feature taxonomy.
- No general buffering until syntax becomes unambiguous.
- Output is immediate and irreversible by default.
- Small, targeted buffers remain allowed for specific high-value constructs.
- Block and inline behavior may remain in one coordinated state machine.
- CommonMark examples and the reference implementation remain the compatibility oracle.
- Exact compatibility is not always possible when interpretation depends on unseen input.

The intended model is:

```text
normalized token + current state
    ↓
deterministic transition
    ↓
new state + irreversible output actions
```

---

# 2. Core refactor considerations

## 2.1 Make the parser an explicit incremental transducer

Centralize parser behavior around:

```text
step(state, token) -> new_state, actions
```

Example actions:

```text
EmitText(text)
OpenStyle(style)
CloseStyle(style)
EmitLineBreak(kind)
OpenContainer(type)
CloseContainer(type)
SuppressToken
```

The parser should produce actions rather than writing ANSI directly. ANSI generation remains a separate action consumer.

This does not require separating block and inline parsing. It only requires making state transitions and output decisions observable.

Important requirements:

- State mutations should happen through explicit transitions.
- Helpers should return decisions rather than silently emitting output.
- Token processing should be deterministic.
- The same state and token must produce the same transition.

This is the foundation for all other improvements.

---

## 2.2 Diagnose failures by the first irreversible divergence

For each failed example, find the first token where emitted output becomes incompatible with the CommonMark result.

Record:

```text
input position
normalized token
state before transition
selected transition
state after transition
emitted actions
reference-required result
```

Every divergence should be classified into one of three cases:

### Missing state

The required information was already present in consumed input, but the parser did not preserve it.

**Action:** add or correct the relevant state.

### Incorrect transition

The state contained enough information, but the parser selected the wrong behavior.

**Action:** correct the local transition.

### Future-dependent decision

The consumed prefix does not contain enough information to choose the CommonMark interpretation.

**Action:** retain the streaming policy, introduce a targeted buffer, or accept the incompatibility.

This prevents changes to parser logic when the real limitation is irreversible streaming.

---

## 2.3 Use CommonMark as a test oracle, not as the production architecture

Continue using CommonMark examples, but supplement their expected HTML with the reference implementation where useful.

The test system may inspect the reference AST or normalized semantic result. The production parser should not adopt that architecture.

Compare:

```text
CommonMark input
    ├── reference parser → expected structural/visible result
    └── streaming parser → actual actions and ANSI output
```

The comparison should distinguish:

- construct-recognition errors;
- rendering errors;
- mismatches that are unavoidable under immediate output.

This avoids treating every failed CommonMark example as the same kind of parser bug.

---

## 2.4 Minimize and cluster failures by parser behavior

Do not organize difficult failures primarily by Markdown features such as “list,” “heading,” or “link.” Their interactions are too numerous.

For each failure:

1. Reduce it to the smallest input that preserves the same first divergence.
2. Record the transition responsible for that divergence.
3. Group examples sharing the same transition or state pattern.

For example:

```text
Cluster: delimiter close while inside link label
Transition: T-417
Affected examples: 14
Reduced input: "[*a*]"
```

Work on clusters rather than processing CommonMark examples sequentially.

A valid parser change should normally improve a cluster or a neighborhood of related inputs—not only one exact example.

---

## 2.5 Make streaming conflicts and targeted buffering explicit

Some CommonMark results cannot be reproduced with immediate irreversible output because the correct action depends on future input.

For each meaningful conflict, record:

```text
indistinguishable consumed prefix
possible future continuations
actions required by each continuation
current streaming policy
examples protected
examples sacrificed
```

Example:

```text
Conflict:
  An opening delimiter may remain unmatched or close later.

Current policy:
  Emit immediately according to the parser’s chosen online behavior.

Alternative:
  Buffer the delimiter region until resolved.

Decision:
  Use buffering only if the affected examples justify the added complexity.
```

Targeted buffers should use a common probe interface:

```text
NotApplicable
NeedMoreInput
Resolved(actions)
Rejected(fallback)
```

Each probe must define:

- trigger condition;
- resolution condition;
- EOF behavior;
- fallback behavior;
- maximum retained input, if applicable.

This keeps exceptional buffering isolated from the main state machine.

---

## 2.6 Protect behavior with transition-aware regression gates

Before accepting a change, report:

```text
transitions changed
tests exercising those transitions
newly passing tests
newly failing tests
unchanged tests
known streaming conflicts affected
```

Use a simple acceptance rule:

```text
No protected regression
and
At least one meaningful improvement
```

If a protected example regresses, the change must either:

- be corrected;
- be expanded into a complete multi-transition fix; or
- be accepted as an explicit compatibility tradeoff.

This replaces uncontrolled example-by-example patching with deliberate behavioral changes.

---

# 3. Refactor phases

## Phase 1: Establish the baseline

- Freeze current behavior in regression tests.
- Mark currently important passing examples as protected.
- Record pass/fail results for all CommonMark examples.
- Separate parser failures from obvious ANSI-renderer failures.

No compatibility changes should be made during this phase.

---

## Phase 2: Introduce the transducer boundary

Refactor token handling into:

```text
step(state, token) -> state, actions
```

- Replace direct ANSI writes with parser actions.
- Make important parser state explicit.
- Ensure existing tests produce the same visible output.
- Keep existing special-case buffers initially.

The purpose is observability and containment, not changed behavior.

---

## Phase 3: Add transition tracing and divergence detection

For every test, capture:

- input token sequence;
- important state transitions;
- emitted actions;
- first mismatch with the reference result.

Classify each failure as:

```text
missing state
incorrect transition
future-dependent
renderer-only
```

This becomes the standard diagnostic workflow.

---

## Phase 4: Reduce and cluster the existing failures

- Automatically minimize failing examples.
- Preserve the same first-divergence signature during reduction.
- Cluster failures by responsible transition and relevant state.
- Rank clusters by number and importance of affected examples.

Stop fixing CommonMark examples in numerical order. Fix the highest-value behavioral clusters first.

---

## Phase 5: Correct transitions and isolate justified lookahead

For each selected cluster:

1. Inspect the reduced reproducer.
2. Determine whether consumed input is sufficient.
3. Correct missing state or the local transition where possible.
4. If future input is required, decide between:
   - current online policy;
   - targeted probe/buffering;
   - accepted incompatibility.
5. Generate nearby input variations and compare them with the reference parser.
6. Run the full protected regression suite.

Every accepted streaming tradeoff should be recorded explicitly.

---

## Phase 6: Stabilize and continue compatibility work

Once the architecture is stable:

- keep the transition trace available in debug builds;
- require impact reports for parser changes;
- add every fixed reduced reproducer as a permanent test;
- periodically merge clusters that share the same underlying transition;
- review special-case probes and remove overlapping behavior;
- continue CommonMark compatibility work cluster by cluster.

The goal is not to eliminate all exceptions. It is to prevent exceptions from modifying unrelated behavior invisibly.

---

# What this redesign accomplishes

It does not make Markdown clean, because Markdown is not clean. It changes the development process from:

```text
Example fails
→ modify broad parser condition
→ unrelated regressions
→ add more conditions
```

to:

```text
Example fails
→ locate first irreversible divergence
→ minimize it
→ cluster by transition signature
→ determine past-state bug versus future-information conflict
→ change one transition or record a streaming policy
→ test generated neighborhood
→ reject any unexplained protected regression
```

The central redesign is therefore:

> Keep the parser as an immediate streaming state machine, but turn it into an observable deterministic transducer. Use first-divergence analysis, reduction, transition-based clustering, and explicit streaming tradeoffs to control compatibility work.

This avoids imposing a conventional AST-based Markdown design while directly addressing the growing cost and regression risk of example-driven implementation.
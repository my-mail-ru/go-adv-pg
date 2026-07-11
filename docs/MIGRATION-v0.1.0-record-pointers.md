# Migration: ActiveRecord records are passed by pointer

Applies when upgrading to go-adv-pg v0.1.0 or later (records were passed by value through v0.0.11).

## What changed

For every table with ActiveRecord enabled (`DisableActiveRecord` not set), the generated API now
passes records exclusively by pointer:

| Generated method                        | Before                 | After                   |
|-----------------------------------------|------------------------|-------------------------|
| unique-index Select                     | `(XxxRecord, error)`   | `(*XxxRecord, error)`   |
| non-unique / multi-key Select           | `([]XxxRecord, error)` | `([]*XxxRecord, error)` |
| InsertMulti / UpdateMulti / DeleteMulti | `[]XxxRecord`          | `[]*XxxRecord`          |

Unchanged: `Model.Record()` already returned `*XxxRecord`; `Insert`, `Update`, `FullUpdate` and
`Delete` already took `*XxxRecord`; tables with `DisableActiveRecord: true` keep value semantics.

Behavior note: a unique-index Select now returns `nil` on error (including `sql.ErrNoRows`)
instead of a zero-value record.

## Why

A Record is a live object — it carries change-tracking state, mutator deltas, and (with
`EnableLock`) a `sync.RWMutex` — and its only constructor already returned a pointer. The
value-based selectors and Multi methods forced copies (`*(Model{...}.Record())` to build slices,
lost-mutation `for _, r := range results` loops) and made `EnableLock` unimplementable: a struct
holding a mutex must not be copied (`go vet` copylocks). With uniform pointer semantics,
`EnableLock` changes no generated signatures — turning locking on or off is a one-flag change
with no call-site rewrite.

## Migrating

Update the dependency, run `go generate ./...`, and fix the resulting compile errors — they are
mechanical. Then run `go vet ./...` and your tests. To have an AI assistant do it, use the prompt
below.

## AI migration prompt

Paste this prompt into your AI coding assistant, run from the root of a repository that uses
go-adv-pg:

````text
Migrate this repository from go-adv-pg's records-by-value API to the records-by-pointer API
(go-adv-pg > v0.0.11).

Background — for every go-adv-pg table with ActiveRecord enabled (its advpg.Table definition does
NOT set DisableActiveRecord: true), the generated API changed:

- unique-index selectors: `(XxxRecord, error)` -> `(*XxxRecord, error)`; on error (including
  sql.ErrNoRows) the returned pointer is now nil instead of a zero-value record;
- non-unique and multi-key selectors: `([]XxxRecord, error)` -> `([]*XxxRecord, error)`;
- InsertMulti / UpdateMulti / DeleteMulti parameters: `[]XxxRecord` -> `[]*XxxRecord`.

NOT changed: `Model.Record()` (already returned `*XxxRecord`), Insert/Update/FullUpdate/Delete
(already took `*XxxRecord`), and every table with `DisableActiveRecord: true` (still value-based —
leave those call sites alone).

Steps:

1. Update the go-adv-pg module dependency, then run `go generate ./...` to regenerate all
   `*_generated.go` files.

2. Compile everything, including all test build tags (`go build ./...`, `go vet ./...`,
   `go vet -tags=<your tags> ./...`), and fix every error mechanically:
   - `rec, err := dao.SelectByX(...)`: rec is now `*XxxRecord`; getter/setter calls on it stay
     unchanged. Remove now-invalid address-taking at use sites: `dao.Update(ctx, &rec)` ->
     `dao.Update(ctx, rec)`.
   - `var rec XxxRecord` later assigned from a selector -> `var rec *XxxRecord`.
   - Slice types that feed or receive DAO methods: `[]XxxRecord` -> `[]*XxxRecord`, including
     `make([]XxxRecord, ...)`, composite literals, struct fields, map values, and channel types
     that carry them.
   - Constructor deref-copies: `*(Model{...}.Record())` -> `Model{...}.Record()`;
     `recs[i] = *m.Record()` -> `recs[i] = m.Record()`; `append(recs, *m.Record())` ->
     `append(recs, m.Record())`.
   - `&recs[i]` passed to Insert/Update/Delete where recs became `[]*XxxRecord` -> `recs[i]`.
   - Sort/compare/filter callbacks over result slices: `func(a, b XxxRecord)` ->
     `func(a, b *XxxRecord)`.

3. Flag — do NOT silently "fix" — each of the following for human review, with file:line and a
   suggested resolution:
   - Copies used as snapshots, e.g. `old := rec` or `old := *rec` taken to compare before/after
     state. With pointers this aliases the same object instead of copying; there is no exported
     way to deep-copy a Record. Suggest re-selecting from the database or capturing the needed
     field values into locals before mutating.
   - Unique-selector call sites that use the returned record even when err != nil (previously a
     zero-value record, now a nil pointer that would panic). Add or verify the error check.
   - Records compared with `==` or `reflect.DeepEqual`, stored by value in maps/slices that are
     mutated later, or sent by value over channels — review the aliasing implications.

4. Never edit `*_generated.go` by hand, and don't touch call sites of tables with
   DisableActiveRecord.

5. Finish with `go build ./...` and `go vet ./...` for all build-tag combinations (vet must be
   clean — for EnableLock tables it reports any remaining record copies), run the test suite, and
   report a summary: what was changed mechanically, plus every site flagged in step 3.
````

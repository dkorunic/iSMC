---
name: validate-temp-mappings
description: Use when the user asks to review/validate/audit temperature sensor mappings against raw SMC report dumps in internal/reports/, propose fixes for mapping issues, or check whether src/temp.txt is consistent with the dumps. Invoke whenever a request mentions reviewing sensor mappings, finding mapping bugs, or validating per-platform sensor coverage.
---

# Temperature Sensor Mapping Validation

Cross-check `src/temp.txt` against the raw SMC dumps in `internal/reports/`. The mechanical part is a Go test; your job is the judgement on top of it. Collect evidence and propose categorized fixes — do not edit until the user confirms.

## Pipeline

- `src/temp.txt` — the only file to edit for mapping changes. `Description:KEY:Platform`, `//` comments. The header documents the format.
- `smc/sensors.go` — generated. After editing `temp.txt`: `cd smc && ./gen-sensors.sh sensors.go` (or `task generate`). A stale `sensors.go` passes nothing: the replay test reads the generated table.
- `smc/smc.go` — `platformMatches` (tag matching), `forEachKey` (wildcard expansion), `addGeneric` (last-write-wins), `isValidReading` (plausibility window, `minTempCelsius`–`maxTempCelsius`).
- `smc/conv.go`, `smc/rawtemp.go` — byte decoding. `RawKeyToFloat32` is the temperature-only decoder; `getKeyFloat32` also serves current/voltage/power, so temperature bounds do not belong there.
- `internal/platform/mapping.go` — `products` (`hw.model` → family + SKU) and `skuLayouts` (per-SKU P/E/S core ranges, pair signature, die count). **This is the roster; do not keep a copy elsewhere.** Unknown model IDs fall back to the CPU brand string (`productFromBrand`).
- `internal/reports/report-<family>[-pro|-max|-ultra][-<seq>].txt` — one dump per machine. The filename is the only SKU label a dump has; lines are `KEY  [type]  [decoded] (bytes …)`.

### Semantics that shape every finding

- **Platform tags**: `All`; `Apple` (any Apple Silicon); a family (`M4` = every M4 SKU); a SKU spelled exactly as `Product.CPU` (`M4 Pro`, `M3 Max`, `A18 Pro`); or `<family> Base` for the base SKU only (`M4 Base`), because the base SKU's name equals the family. Prefer the narrowest tag the dumps support; keep a family tag when a SKU without a dump might need the row.
- **Last-write-wins**: every key sharing a description collapses to one output row holding the last valid read. Triplets rely on this to surface the third key. It also hides a hotter sensor whenever two unrelated keys share a label.
- **Wildcard `%`** expands to digits 0–9 only. Letter slots (`TPDA`, `TRDa` …) need explicit rows.
- **Sentinels**: inactive slots read 0, small negatives, or junk up to ~14.8 °C and are dropped by the floor (`minTempCelsius`, 15 °C). Genuine probes go down to ~16.7 °C in cool rooms, so the floor has almost no margin — never argue from "below 21 °C means sentinel". Keys pinned at exactly **40.00 °C** are clock-gated placeholders — valid to the filter, meaningless as data. Idle M4 Pro/Max park every P-core triplet there.
- **Legacy `All` rows** come from Intel-era key lists and also load on Apple Silicon, where the same key can mean something else (`TG%B` is a battery sensor, `Tm2P` an M5 memory slot). When an `All` row is live in an Apple Silicon dump, check its label against neighbouring keys; if it is wrong there, retag the row `Intel` and add an `Apple` row.
- **Case matters**: `TVMR` ≠ `TVMr`, `TsOP` (letter O) ≠ `Ts0P`.

### Apple Silicon conventions

- Core types: **P** performance, **E** efficiency, **S** super (M5 only). `skuLayouts` says which two a SKU has.
- **M1–M4, A18**: per-core sensors are triplets (probe A, probe B, third). Under load the third is the max; at idle it can sit below probe B, so "third is max" is a tendency, not a rule. A constant B − A offset across a whole dump means B is derived from A, not a separate probe.
- **M5**: one key per core. `Tp0?` at stride 4 covers S-cores in the low range and P-cores further up; type is positional, not by prefix. Never look for A/B/max companions on M5.
- Prefixes: `Tp*` P (or S+P on M5), `Te*` E, `Tg*` GPU, `Tm*` memory, `Ts*` SSD, `Tf*` GPU fabric, `Tex`/`Tpx` cluster dies, `TUD*` uncore, `TPD*` power delivery, `TRD*` RF delivery, `Ta*` ambient. Suffix `X` is an aggregate max.
- Ultra SKUs are two Max dies: expect die-level families in two parallel sets, and flag a mapping that covers only die 0.
- A dump whose keys contradict its filename (Intel fingerprints `Tc0a`, `TIOP`, `Tarc` in an `m*` file; eight live P triplets on a base SKU) is mislabelled — rename it with `git mv` before trusting any finding from it.

## Procedure

### 1. Run the replay test

```sh
CGO_ENABLED=1 go test ./smc/ -run Test_DumpReplay -v
CGO_ENABLED=1 go test ./smc/ -run 'Test_DumpReplay/m4-pro$' -v   # one dump
```

`smc/replay_test.go` decodes every `T*` key of every dump from its bytes, resolves it through the production mapping for that dump's family and SKU, and checks:

- **cores** — live `CPU {Performance,Efficiency,Super} Core N` rows fall inside the `skuLayouts` range;
- **collide** — no description hides a hotter key (> 0.5 °C) behind the last-written one;
- **unmapped** — every live key has a description;
- **dupkey** — no key in scope has two descriptions (both rows would be emitted).

A core whose live keys all read exactly 40.00 °C does not count towards **cores**. Failures read `new violation (sku X): …` or `stale allowlist entry, remove it: …`.

Two table-wide tests sit next to it and run with `go test ./smc/`:

- `Test_DumpLiveRows` — every row not tagged `All`/`Intel` reads live in at least one dump its tag matches. Exceptions are in `deadRowAllow`, each backed by evidence outside the dumps (currently the M4 Pro 14-core fixture in `smc/mapping_test.go`). Before deleting a "dead" row, grep the fixtures in `mapping_test.go` for its key.
- `Test_SensorRowLint` — platform tags are known families/SKUs, keys are four characters, wildcards pair up.

### 1a. Triage the dumps

Before trusting a dump, count its gated keys (`grep -c 'bytes 00 00 20 42' internal/reports/report-<name>.txt`) and note whether it is idle or loaded. A dump with dozens of 40.00 keys says nothing about the cores it parks; only an ungated dump verifies a per-core mapping.

Coverage decides how strong a finding can be. No dumps: base M1, M3; M2 Ultra, M3 Ultra. One dump: base M2 (idle), base M5 (idle), A18 Pro, Intel (T2 only), M1 Max, M1 Ultra, M2 Pro (Mac14,10, loaded), M2 Max (sentinels only), M4 Max (gated). Base M2 and M2 Pro/Max use different key layouts. Label findings on those SKUs as unverified. Base M5 S/E-core rows are positional guesses from that idle dump.

### 2. Read the allowlist

`replayAllow` in the same file is the list of known, tolerated violations, keyed by dump name, as `cores <P|E|S>=<n>`, `collide <description>`, `dupkey <key>` or `unmapped <key> …`, each with a reason comment. It is the backlog. A green test means "no regressions", not "no problems": every entry is a candidate finding, and its comment says what evidence was missing last time.

For a new dump: add the file, run the test, and treat each new violation as either a `temp.txt` fix or a new allowlist entry with a reason.

### 3. Judge what the test cannot

- **Is an allowlisted gap now fixable?** A new dump of the same SKU (especially a loaded one against an idle one) is usually what turns "cannot tell" into evidence.
- **Aliases.** Keys that match byte-for-byte across independent dumps are one register; map one and allowlist the rest. Compare decoded bytes, not the inline value.
- **Mislabelled aggregates.** A "Core N" beyond the SKU's core count that still reads a real value is normally a cluster aggregate. On M5, check that no `Tp0?` slot is labelled both Super and Performance, and that no row treats `Tp00/01/02` as a triplet.
- **Counts beyond cores.** GPU clusters, memory channels and SSD slots follow the same pattern but are not checked by the test.
- **Decode overrides.** Any key-specific decode special case must be validated against dump bytes for that key across every dump. A fixed-point reading that lands on values like 102.4 or 51.2 is the mantissa of a float, not a temperature.
- **Coincidence or alias?** With only idle dumps of a SKU, many keys match byte-for-byte by quantisation alone. Call two keys one register only when a loaded dump agrees.
- **Label honesty.** Where the physical meaning is unknown, use a generic label that says so. Never assign a core number the dumps do not support, and never reuse a description that already resolves to a different live key on the same SKU.

On live hardware, `smc.EnumerateKeysPrefix(conn, "T")` lists every temperature key directly, including ones no dump has captured.

### 4. Categorize and propose

| Severity | What | Fix shape |
| --- | --- | --- |
| 1 — Wrong/misleading | Bad label, phantom core, hidden hotter sensor, mislabelled dump | `temp.txt` edit or `git mv` |
| 2 — Convention violation | Broken triplet, wrong count, decode garbage | Line edit, retag to a SKU, or decoder fix |
| 3 — Silent data loss | Live key or key family with no mapping | Additive rows |
| 4 — Cosmetic | Comment drift, harmless duplicate | Optional |

For each issue cite `temp.txt` line numbers, name the dumps that prove it with concrete values, and give the exact replacement text. Say which allowlist entries the fix removes. Renaming a description changes user-visible output (and InfluxDB series names) — call that out.

### 5. Apply (only after the user confirms)

1. Edit `src/temp.txt`; regenerate `smc/sensors.go`.
2. Remove the `replayAllow`/`deadRowAllow` entries the fix clears; add entries with reasons for anything deliberately left.
3. `CGO_ENABLED=1 go test ./...` — `Test_DumpReplay` plus the per-SKU tests in `smc/mapping_test.go` (M4 Pro, A18 Pro, M2 Pro, M5 Pro), which encode earlier decisions; changing one needs a stated reason.
4. `task lint` — no new issues in files you touched.

## Output shape

```
# Temperature Sensor Mapping Validation — Findings

## Severity 1 — Factually wrong / misleading
### 1.1 <issue title>
**Evidence:** <dump names + decoded values>
**Fix:** <exact before/after lines; allowlist entries removed>

## Severity 2 — Convention violations
## Severity 3 — Unmapped key families
## Severity 4 — Cosmetic

## Recommended order of operations
1. <lowest-risk fix first>
```

End with a single question: **"Want me to apply any of these?"**

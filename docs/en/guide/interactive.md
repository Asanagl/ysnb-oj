# SPJ / interactive problems

## SPJ (special judge)

- The checker is a single C++17 source file, saved from the problem editor;
  the judge caches compiled artifacts by sha256.
- Calling convention (testlib style, the common DOMjudge/Kattis-compatible form):

```
./checker <input> <answer> <output>
exit 0        -> AC
exit != 0     -> WA (first stderr line is shown to the user as feedback)
```

## Interactive problems

- The interactor is a single C++17 source file; the test input is passed via argv:

```
./interactor <input>
```

- The user program's stdin/stdout is piped straight to the interactor's
  stdout/stdin. Both run inside sandboxes (the interactor is also network-jailed
  and resource-limited).
- **Exit code convention**:
  - `0`: the user passes; if the user program itself times out or crashes, the
    user-side status wins.
  - `1`: WA (first stderr line as feedback).
  - Anything else: SE (system error — check the judge logs).
- Deadlock protection: when either process exits, the daemon kills the other
  immediately; the whole pair is bounded by a wall-clock watchdog (user limit =
  problem limit × language multiplier; the interactor gets a fixed 30 s).
- A case zip may omit `.out` files (the interactor judges correctness itself).

## Toolchain interop

Problem package import/export (problem admin → "Import package" / editor
→ "Export zip"):

- **Native format** (what export produces): `statement.md` + `meta.txt` +
  `testdata/` + `checker.cpp`/`interactor.cpp` (SPJ/interactive sources are
  included automatically and the judge mode is restored on import).
- **DOMjudge packages**: parses `problem.yaml` (name/title, limits.timeout,
  limits.memory), `statements/*.md` (.md only — TeX/PDF prompts a manual
  statement), `.in/.ans` from `data/sample` + `data/secret` (nested dirs are
  flattened and renumbered). `validator/checker` sources are imported as the
  checker — but note DOMjudge validators and this OJ's checkers use
  **different argument conventions** (this OJ: `./checker <in> <ans> <out>`,
  exit 0 = AC); verify manually after import.
- **Hydro packages**: `problem.yaml` (title/time/memory) + `problem[_zh].md`
  + `testdata/*.in/.out|.ans` + `check/checker.cpp` (imported as spj).
- All three formats are auto-detected; cases are renumbered by natural filename
  sort. `.in` files without an answer are skipped for the whole package
  (interactive problems should use the native format, or upload cases after
  import).
- checker/interactor can also be uploaded as `.cpp` files in the editor
  (contents land in the text box and save with the form).

## Relation to IOI scoring

The IOI score table is orthogonal to the checker — the checker still only
returns AC/WA; partial credit comes from the per-case score table
(case-scores).

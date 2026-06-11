# NeverScript (THUG2 fork) — Known Limitations & Future Work

This fork patches NeverScript v0.9 to decompile/recompile **Tony Hawk's Underground 2
(PC)** QB scripts into bytecode the retail game loads. It is validated end-to-end
in-game (an unmodified recompile of `AU_sfx.qb` loads Australia identically to the
original; a real edit — silencing the seagull fly-up cry — works as intended).

This document tracks what is *not* yet perfect.

## 1. Verification hygiene / byte-identity
**Status: ACHIEVED for `AU_sfx.qb`; near-identical for `AU.qb`.**

`recompile(decompile(AU_sfx.qb)) == AU_sfx.qb` is now **byte-identical** (and a stable
fixpoint). The four cosmetic sources were fixed: the decompiler's header comment
(dropped — it injected a leading `0x01`), the `__register_checksums__` directive's
surrounding newlines (removed — they added stray `0x01`s before the table), and the
per-random **branch-0 newline** (now preserved via a newline right after `random {`
and an offset-base that follows it, instead of force-emitting it on every random).

`AU.qb` (478 KB) round-trips to within **1 byte** — a single newline-after-`=` (top-
level array assignment) that the compiler drops. Not yet byte-identical; harmless.
`AU_Scripts.qb` can't be measured for byte-identity yet because it fails to recompile
(see §2).

## 2. Decompiler opcode / construct coverage
**Status: open — the main real gap.**

Known coverage gaps (all pre-existing, independent of the byte-identity work):
- **`AU_Scripts.qb` recompile fails** — "Unrecognised token in body of code" around
  line 1978 of its decompiled `.ns` (a `while { if SkaterCamAnimFinished Name=... {
  break ...` region). It *decompiles* fine; the *compiler* can't re-parse some
  construct there. This is the highest-value gap to close for full level coverage.
- **`scripts\game\menu\gamemenu_levelselect.qb` fails to decompile** — unhandled
  `0x0e` in a context the body walker doesn't expect (a menu script, not a level).

Broadening coverage is best done **reactively**: when a needed script won't
decompile/recompile, fix the specific opcode/construct it trips on (the same way the
level-script opcodes were added).

## 3. In-game validation scope
**Status: open.**

Only **Australia (AU)** is validated in-game. Other levels almost certainly work (same
opcode set) but are untested end-to-end.

## 4. Adding brand-new symbols in a mod
**Status: open / design constraint.**

THUG2 appends a trailing symbol/name table. We reproduce the **original's exact order**
verbatim (via the `__register_checksums__` directive) — byte-faithful for round-trips
and value-mods. But for a mod that introduces a **brand-new** symbol not in the
original table, we cannot compute its correct slot, because the order is a Neversoft
hash-bucket dump whose hash we have not reproduced (no simple modulo/mask matches).
Note: it is also not confirmed whether table order matters at runtime at all — the
one data point suggesting it did was later traced to an unrelated broken save file.

## 5. Engine is out of scope
The game **engine** (`THUG2.exe`: rendering, physics, the QB interpreter, low-level
systems) is compiled C++ and can only be disassembled, never recovered as source.
Only the **script layer** (`.qb` files) is decompilable to NeverScript.

## Pre-existing repo issues (untouched, do not affect the `ns` binary)
- `compiler/tests` has two `main`s (won't `go test`).
- The unused `newcompiler` package has a `Sprintf` build error.
- The consistency test requires `roq.exe`.

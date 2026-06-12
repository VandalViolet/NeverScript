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

Known coverage gaps:
- **`AU_Scripts.qb` recompile — FIXED.** It used to fail ("Unrecognised token in
  body of code") on `if <function-call condition> { break }`: the condition's
  invocation greedily tried to absorb the `{ break }` body as a struct argument, and
  `ParseStruct` choked on `break`. Fixed by bailing out of struct parsing on `break`/
  `return` (control-flow, never struct content) the same way `if`/`while` already do.
  AU_Scripts now recompiles to semantically-equivalent, stable-fixpoint bytecode.
- **`scripts\game\menu\gamemenu_levelselect.qb` fails to decompile** — unhandled
  `0x0e` in a context the body walker doesn't expect (a menu script, not a level).

Broadening coverage is best done **reactively**: when a needed script won't
decompile/recompile, fix the specific opcode/construct it trips on (the same way the
level-script opcodes were added).

## 2b. `switch` does not round-trip byte-identically
**Status: open / known design constraint.**

The compiler has no native `switch` opcode, so the decompiler **lowers** `switch`
statements to equivalent `if/elseif` chains (valid for THUG2's switch-on-variable
cases, e.g. `<Difficulty_Level>`/`<TinCans>`). This is semantically equivalent and a
stable fixpoint, but **not byte-identical**: a file with switches (e.g. AU_Scripts.qb,
+307 bytes) recompiles larger, with if-chain opcodes instead of the compact `0x3c`
switch. Byte-identity for such files would require teaching the compiler to emit
native `switch` bytecode. (The recompiled if-chain form is runtime-safe by
construction — it uses the same short-if opcodes proven in-game on AU_sfx — but has
not yet been in-game-validated for AU_Scripts specifically.)

## 2d. Decompiler: front-end / cutscene / level scripts (the "0x0e" gap)
**Status: switch-on-expression FIXED; more gaps open.**

THUG2's front-end scripts use parenthesised-expression forms the decompiler didn't handle,
blocking `mainmenu_scripts.qb`, `cutscene.qb`, `Levels.qb`, `gamemenu_options.qb`,
`gamemenu_levelselect.qb`, `gamemenu.qb`.
- **FIXED — `switch (<expr>)`:** the switch decoder only accepted a bare checksum as the switch
  value; now it falls back to `DecompileExpression` when the value is `(…)`. This unlocked
  **decompilation** of `cutscene.qb`, `Levels.qb`, `gamemenu_options.qb`. Bare-checksum switches
  are byte-identical to before (AU_Scripts etc. unchanged); full shipped corpus round-trips
  byte-identically (no regression).
- **OPEN — array subscript on an expression** `(<a>.<b>)[<c>]` (Byte_Array 0x05 as a postfix):
  blocks `gamemenu_levelselect.qb`, `gamemenu.qb`.
- **OPEN — `mainmenu_scripts.qb`:** a short-if body begins with a `0x00` byte (read as EndOfFile),
  so the body bails and the short-if can't find its EndIf. Cause unclear (padding? misread). 
- **Note:** decompiling ≠ recompilable. `cutscene.qb` decompiles but RECOMPILE fails (a compiler
  "Unrecognised token" gap); `Levels.qb`/`gamemenu_options.qb` recompile only via the §2b
  switch→if-chain lowering (semantically-equivalent, not byte-identical, needs in-game validation).

## 2c. Chained parenthesised expressions `(A op B op C ...)` — FIXED
**Status: FIXED (byte-identical).**

The compiler used to parse only a *single* operator inside parentheses: `(A = B)`
worked, but `(A = 0 or B = <c>)` failed ("Unrecognised token in body of code") and
`(0.8 * 250.0 / <width>)` failed ("Incomplete assignment"). `handleBinaryOperator`
parsed one operator, then required `)`; anything else aborted. This blocked common
goal-NPC and UI scripts (e.g. `scripts\game\ped\Sk6Ped_StateLogic.qb`,
`scripts\game\menu\soundoptions.qb`).

THUG2 stores a parenthesised expression as one flat infix token stream between a single
`0xE`/`0xF` pair (operands and operator bytes inline, no nested parens). The fix parses
the whole chain into a new `AstKind_FlatExpression` (ordered operands + operator kinds)
and emits it verbatim. Because `ParseExpression` already grabs `or`/`and` as a
right-associative postfix, the parser **flattens that logical spine** back into the
flat lists so mixed comparison/logical chains reassemble correctly. A single operator
still yields the original binary node (byte-identical to before — zero regression);
two or more yield the flat node. Operator→byte map: `=`0x7, `<`0x12, `<=`0x13, `>`0x14,
`>=`0x15, `-`0xA `+`0xB `/`0xC `*`0xD, `or`0x32, `and`0x33 (`FlatOperatorByte`).

Validated: `(A = 0 or B = <c>)` and `(0.8 * 250.0 / <width>)` compile **byte-identical**
to the originals; the full `Sk6Ped_StateLogic.qb` now round-trips **byte-identically**
(previously failed to recompile); AU_sfx/menu round-trips unchanged. Files: `compiler/
ast.go` (`AstKind_FlatExpression`), `compiler/parser.go` (chaining + spine flatten),
`compiler/output.go` (`FlatOperatorByte` + emitter).

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

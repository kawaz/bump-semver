# `vcs outdated` Test Coverage Matrix (v0.31.0)

> Status: living document. Source-of-truth for the decision-table cells that
> the test suite pins. Each cell maps to a test function name; uncovered
> cells are scoped-out with an explicit reason.
>
> Scope: v0.31.0 release artifact = `src/glob_backref.go`,
> `src/glob_backref_test.go`, `src/cmd_vcs_outdated.go`,
> `src/cmd_vcs_outdated_test.go`. See spec `docs/specs/glob-backref-v0.1.0.md`
> and DR-0028.

## 1. Axes

The decision table is the product of these axes. Not every cartesian cell is
meaningful — see §3 for the cells we *intentionally* skip.

### FROM axis (pattern shape)

| Tag | Meaning |
|---|---|
| `lit` | literal FROM (no `glob:` prefix) |
| `g*` | `glob:` with `*` only |
| `g**` | `glob:` with isolated `**` (recursive) |
| `g**!` | `glob:` with non-isolated `**` (= degrades to `*`, §2.2.1) |
| `g[]` | `glob:` with char class |
| `g{}` | `glob:` with single-level brace (1-axis Cartesian) |
| `g{}x{}` | `glob:` with multi-axis brace |
| `g{,}` | `glob:` with empty alt branch (§2.2.4) |
| `g?` | `glob:` containing `?` (= MVP reject) |
| `g{{}}` | `glob:` with nested `{}` (= MVP reject) |
| `g[^]` | `glob:` with complement char class (= MVP reject) |
| `g:empty` | `glob:` body empty |

### FROM match cardinality

| Tag | Meaning |
|---|---|
| `m0` | 0 matches |
| `m1` | exactly 1 match |
| `mN` | ≥2 matches |
| `m**0seg` | `**` matches 0 segments at root |

### TO axis (template shape)

| Tag | Meaning |
|---|---|
| `t-lit` | literal TO, no backref |
| `t-$N` | `$1`..`$9` backref |
| `t-${N}` | `${1}` braced backref |
| `t-${10}` | `${10}` 2-digit braced backref |
| `t-$10!` | bare `$10` (= ambiguous, MVP reject) |
| `t-${name}` | `${abc}` named (= v0.2 reject) |
| `t-${}` | empty `${}` (= reject) |
| `t-$trail` | trailing `$` (= reject) |
| `t-{}` | TO with `{}` brace (mandatory expansion) |
| `t-glob:*` | `glob:` TO with own wildcard (= optional 2nd-stage walk) |
| `t-empty` | empty TO string |
| `t-$0` | `$0` full-path back-reference |

### Flag axis

| Flag | States | Pinned? |
|---|---|---|
| `--strict` | on/off | both |
| `--explain` | on/off | both |
| `--strict --explain` | on | yes (explain wins; characterization) |
| `--glob-dotfile=true/false` | both | dotfile branch covered |
| `--glob-gitignored=true/false` | both | covered |
| `--glob-ignorecase` | on/off | only off covered (= default) |
| `--vcs git` / `--vcs jj` / auto | three | git + jj covered; auto inherits |

### Pair-shape axis

| Tag | Meaning |
|---|---|
| `p1` | single pair (no `--`) |
| `pN` | N pairs, `--` separated |
| `p-lead--` | leading `--` (= same as `pN`) |
| `p-trail--` | trailing `--` (= empty trailing group, ignored) |
| `p-only--` | only `--` (= usage error) |
| `p-same-from` | two pairs with same FROM (= independent eval) |
| `p-err2` | error in pair 2 (= short-circuit) |

### Freshness/status axis

| Tag | Meaning |
|---|---|
| `fresh` | derived ts ≥ source ts |
| `stale` | derived ts < source ts |
| `missing` | derived path absent on disk |
| `untracked` | derived on disk but no VCS ts (= ts=0) |
| `lit-miss` | literal FROM matched zero (= silent-typo gate) |
| `glob-m0` | glob FROM matched zero (= non-error) |

### Backend / VCS error axis

| Tag | Meaning |
|---|---|
| `b-git` | git backend (= default fixture) |
| `b-jj` | jj backend |
| `b-auto` | `--vcs auto` (= default; covered indirectly by git fixtures) |
| `b-noVCS` | no VCS at all (= exit 3) |
| `b-wrongVCS` | `--vcs git` in non-git dir (= exit 3) |

### Exit code axis (output)

| Code | Meaning |
|---|---|
| 0 | fresh OR `--explain` (overrides everything per current impl) |
| 1 | stale OR missing OR untracked OR `--strict` lit-miss |
| 2 | usage / pattern syntax |
| 3 | VCS backend error |
| panic | grammar drift (= internal invariant break) |

## 2. Matrix table (= pinned cells)

Cell legend:

- *test name*: pins the cell
- *(library)*: pinned at `glob_backref_test.go`
- *(cmd)*: pinned at `cmd_vcs_outdated_test.go`

### 2.1 Existing pre-v0.31.0 cells

| Cell | Description | Test |
|---|---|---|
| C1 | `g*` + `**` + `t-$N` (bundle case) | `TestGlobBackref_T1_Bundle` *(library)*, `TestRun_VcsOutdated_T1_Bundle` *(cmd)* |
| C2 | `lit` FROM + `t-{}` (translation) | `TestGlobBackref_T2_Translation`, `TestRun_VcsOutdated_T2_Translation` |
| C3 | `g**` + multi-segment + `t-$N` (codegen) | `TestGlobBackref_T3_Codegen` |
| C4 | `g{}` Cartesian + `t-$N` | `TestGlobBackref_T4_BraceExpansion` |
| C5 | `g**` `m**0seg` + `t-$N` → `.` → path.Clean | `TestGlobBackref_T5_ZeroSegmentDoubleStar`, `TestRun_VcsOutdated_LeadingSlashDogfood` |
| C6 | backref numbering (4 slots) | `TestGlobBackref_T6_BackrefOrder` |
| C7 | `g{,}` empty alt (§2.2.4) | `TestGlobBackref_T7_EmptyBranch` |
| C8 | TO literal embed: value with `{}` (non-glob TO) | `TestGlobBackref_T8_LiteralEmbed`, `TestRun_VcsOutdated_TOReGlobLiteralEmbed_NonGlobTO` |
| C9 | Grammar drift panic | `TestGlobBackref_T9_GrammarDriftPanic` |
| C10 | `$10` ambiguous reject | `TestGlobBackref_T10_DollarTenAmbiguous` |
| C11 | `${10}` accepted, out-of-range → empty | `TestGlobBackref_T11_BracedTenAccepted` |
| C12 | Cartesian explosion (3³=27) | `TestGlobBackref_T12_CartesianExplosion` |
| C13 | Brace-leak invariant (= doublestar never sees `{`/`}`) | `TestGlobBackref_T13_BraceInvariant` |
| C14 | `{{}}` nested reject | `TestGlobBackref_T14_NestedBraceRejected` |
| C15 | `[^...]` complement reject | `TestGlobBackref_T15_ComplementCharClassRejected` |
| C16 | TO `glob:` escape of captured glob meta | `TestGlobBackref_T16_TOSideGlobEscape`, `TestRun_VcsOutdated_TOReGlobLiteralEmbed` |
| C17 | TO `glob:` + brace expansion | `TestGlobBackref_T17_TOSideGlobWithBraceExpansion` |
| C18 | dogfood ja → en sync (multi-source) | `TestGlobBackref_T18_DogfoodingReadmeJaSourceForOriginal` |
| C19 | Non-isolated `**` collapses to single slot | `TestGlobBackref_NonIsolatedDoubleStarSingleSlot` |
| C20 | `?` reject parser | `TestGlobBackref_RejectQuestionMark`, `TestRun_VcsOutdated_RejectQuestionMarkInFrom`, `TestRun_VcsOutdated_RejectQuestionMarkInTo` |
| C21 | `${N}` ≡ `$N` under escape | `TestSubstitute_BracedEqualsBareUnderGlobEscape` |
| C22 | `--explain` → exit 0 with stale rows | `TestRun_VcsOutdated_Explain` |
| C23 | multi-pair `pN` with mixed stale | `TestRun_VcsOutdated_MultiPair` |
| C24 | mandatory derived missing → exit 1 | `TestRun_VcsOutdated_MissingMandatory` |
| C25 | per-source auto-exclude (= source == derived) | `TestRun_VcsOutdated_AutoExclude` |
| C26 | `--glob-gitignored=false` includes ignored sources | `TestRun_VcsOutdated_GlobFlagsApply` |
| C27 | usage error (1 arg only) → exit 2 | `TestRun_VcsOutdated_UsageError` |
| C28 | `--strict` literal-FROM-not-found → exit 1 | `TestRun_VcsOutdated_StrictLiteralFromMissing` |
| C29 | VCS error (no git repo, `--vcs git`) → exit 3 | `TestRun_VcsOutdated_VCSErrorPropagates` |
| C30 | FROM brace alt captured as `$N` | `TestRun_VcsOutdated_FromBraceCapture` |
| C31 | bare verb → help (no args) | `TestRun_VcsOutdated_NoArgsHelp` |
| C32 | pair splitter cases | `TestSplitOutdatedPairs_Cases` |
| C33 | T1 all fresh (= exit 0) | `TestRun_VcsOutdated_T1_AllFresh` |

### 2.2 New v0.31.0+ cells (added this change)

Library-level (= `glob_backref_test.go`):

| Cell | Description | Test |
|---|---|---|
| L1 | `$0` substitution (full matched path) | `TestSubstitute_Dollar0FullPath` |
| L2 | `${0}` ≡ `$0` | `TestSubstitute_BracedZeroEqualsBare` |
| L3 | `$N` followed by non-digit literal (`$1a`) | `TestSubstitute_DigitFollowedByLetter` |
| L4 | `${N}` followed by digit literal (`${1}0`) | `TestSubstitute_BracedFollowedByDigit` |
| L5 | empty `${}` reject | `TestSubstitute_EmptyBracedRejected` |
| L6 | trailing `$` reject | `TestSubstitute_TrailingDollarRejected` |
| L7 | `$<letter>` reject (non-numeric bare) | `TestSubstitute_NonNumericBareRejected` |
| L8 | `${name}` reject (named) | `TestSubstitute_NamedRejected` |
| L9 | `${999}` accepted, out-of-range → empty | `TestSubstitute_LargeBracedOutOfRange` |
| L10 | Substitute path.Clean: `./x` → `x`, `//` → `/`, `..` collapse | `TestSubstitute_PathCleanApplied` |
| L11 | absolute path (`/foo`) preserved | `TestSubstitute_AbsolutePathPreserved` |
| L12 | escape table (all glob meta: `*?{},[]`) under `glob:` | `TestSubstitute_GlobEscapeAllMeta` |
| L13 | Parser: unterminated `{` | `TestParsePattern_UnterminatedBraceRejected` |
| L14 | Parser: orphan `}` | `TestParsePattern_OrphanCloseBraceRejected` |
| L15 | Parser: unterminated `[` | `TestParsePattern_UnterminatedCharClassRejected` |
| L16 | Parser: orphan `]` | `TestParsePattern_OrphanCloseBracketRejected` |
| L17 | Parser: empty `[]` | `TestParsePattern_EmptyCharClassRejected` |
| L18 | Parser: `[!abc]` BSD-form complement also rejected | `TestParsePattern_BangComplementRejected` |
| L19 | MatchCollect: 0-match returns empty slice (no error) | `TestMatchCollect_ZeroMatchNoError` |
| L20 | MatchCollect: dotfile excluded by default | `TestMatchCollect_DotfileExcludedByDefault` |
| L21 | MatchCollect: `Dotfile=true` includes hidden | `TestMatchCollect_DotfileIncludeFlag` |
| L22 | ExpandPairs: TO `?` rejected | `TestExpandPairs_ToQuestionMarkRejected` |
| L23 | ExpandPairs: empty alt branch on TO | `TestExpandPairs_EmptyBranchOnTo` |
| L24 | `$0` under `glob:` escape | `TestSubstitute_Dollar0UnderGlobEscape` |
| L25 | unselected brace branch backref = "" (§4.2) | `TestMatchCollect_UnselectedBranchBackrefEmpty` |

Cmd-level (= `cmd_vcs_outdated_test.go`):

| Cell | Description | Test |
|---|---|---|
| K1 | `--strict --explain` → exit 0 + stale rows printed *(characterization, see OQ-19)* | `TestRun_VcsOutdated_StrictPlusExplain_ExitZero` |
| K2 | `--strict --explain` + lit-miss → exit 0 *(characterization, OQ-19)* | `TestRun_VcsOutdated_StrictPlusExplain_LitMissExitZero` |
| K3 | `--explain` + missing mandatory → exit 0 with `[missing, will fail]` on stdout *(text/exit contradiction, OQ-20)* | `TestRun_VcsOutdated_ExplainPlusMissing_TextContradictsExit` |
| K4 | `--strict` short-circuits: lit-miss in pair 2 silences stale row of pair 1 *(characterization, OQ-21)* | `TestRun_VcsOutdated_StrictShortCircuitsStaleRow` |
| K5 | FROM `glob:` empty body → exit 2 (pattern err) | `TestRun_VcsOutdated_GlobEmptyBodyRejected` |
| K6 | TO `glob:` empty body → exit 2 | `TestRun_VcsOutdated_ToGlobEmptyBodyRejected` |
| K7 | empty TO string → quietly "fresh" against cwd dir *(silent-green gap, OQ-22; openConcern)* | `TestRun_VcsOutdated_EmptyToCharacterization` |
| K8 | trailing `--` (= empty trailing group ignored, normal exit semantics) | `TestRun_VcsOutdated_TrailingPairSeparator` |
| K9 | leading `--` (= same as no leading sep) | `TestRun_VcsOutdated_LeadingPairSeparator` |
| K10 | only `--` → usage exit 2 | `TestRun_VcsOutdated_OnlyPairSeparator` |
| K11 | multi-pair same FROM (= independent eval, both pairs' rows emitted) | `TestRun_VcsOutdated_MultiPairSameFrom` |
| K12 | pair 2 syntax error short-circuits pair 1 *(characterization, OQ-23)* | `TestRun_VcsOutdated_Pair2ErrorShortCircuits` |
| K13 | untracked derived → exit 1 (non-explain) with `[untracked: ...]` | `TestRun_VcsOutdated_UntrackedDerivedExit1` |
| K14 | untracked derived under `--explain` → exit 0 | `TestRun_VcsOutdated_UntrackedDerivedExplain` |
| K15 | cross-source case (= derived path of A equals path of B): NOT excluded *(characterization, OQ-24)* | `TestRun_VcsOutdated_CrossSourceNotExcluded` |
| K16 | jj backend happy-path stale → exit 1 | `TestRun_VcsOutdated_JjBackendStale` |
| K17 | `--vcs jj` in non-jj dir → exit 3 | `TestRun_VcsOutdated_WrongVcsJjExit3` |
| K18 | glob FROM 0-match (= NOT literal) → exit 0 even with `--strict` | `TestRun_VcsOutdated_StrictGlobZeroMatchExit0` |
| K19 | `$0` in TO yields source path (= guaranteed self-exclude) | `TestRun_VcsOutdated_Dollar0InToExcluded` |
| K20 | `--glob-dotfile=true` reaches sources under dotdir | `TestRun_VcsOutdated_GlobDotfileFlag` |
| K21 | `--glob-ignorecase` + case-different path → **grammar-drift panic** (v0.31.0 bug, OQ-25) | `TestRun_VcsOutdated_GlobIgnorecaseFlag` |
| K22 | `--explain` empty result (no FROM matches) → exit 0 with empty stdout | `TestRun_VcsOutdated_ExplainNoMatches` |
| K23 | `$N` out-of-range in TO → empty literal (= reaches `path.Clean`) | `TestRun_VcsOutdated_OutOfRangeBackrefCleaned` |
| K24 | `$10` in TO (ambiguous) → exit 2 | `TestRun_VcsOutdated_AmbiguousDollar10InTo` |
| K25 | `${abc}` in TO → exit 2 | `TestRun_VcsOutdated_NamedRefInTo` |
| K26 | mandatory `{,}` brace TO with one alt missing → exit 1 *(matches existing C24 but pinned with empty alt)* | `TestRun_VcsOutdated_EmptyAltBraceTOMissingExit1` |

## 3. Scope-out (= cells deliberately not pinned)

| Skipped cell | Reason |
|---|---|
| `g*` + `*` 0-char match | Grammar-drift panic by design (§2.2.3); C9 already pins the panic via synthetic forcing — re-pinning per-real-pattern adds no information. |
| Co-located git+jj backend | DR-0026 boundary; v0.31.0 does not distinguish. Covered indirectly via `--vcs <name>` override; explicit dual-backend layout adds setup cost > value. |
| `--glob-ignorecase=false` (= default) | Already the implicit default of every other cell. |
| `--glob-dotfile=false` (= default) | Same. |
| `--glob-gitignored=true` (= default) | Already implicit (C26 inverse covers the toggle). |
| Cartesian (`g{}x{}`) with 3+ axes beyond C12 | C12 covers the explosion size; per-axis combinatorics adds no semantic info. |
| `$0` + `--strict` literal-miss interaction | Orthogonal — `$0` is substitute-time, `--strict` is FROM-side. |
| `--vcs auto` explicit cell | All git/jj fixtures implicitly exercise `auto`; explicit `auto` is no different. |
| panic recovery from grammar drift inside `vcs outdated` cmd | Spec §7 mandates panic = process exit, intentional; no graceful path to assert. |
| jj `untracked` derived (= jj has no untracked notion) | Backend-specific edge; jj treats all working-copy files as tracked. |
| Symlink / followSymlinks | v0.1.0 spec §15.5 default `false`, not exposed as a CLI flag. |
| Cancellation / streaming | Spec §15.2 explicitly optional, Go MVP scope-out. |

## 4. Coverage stats

- **Existing cells pinned**: C1..C33 = 33 cells (Phase A baseline)
- **New cells added**: L1..L25 (25 library) + K1..K26 (26 cmd) = 51 cells
- **Total cells pinned**: 33 + 51 = **84 cells**
- **Scope-out cells**: 12 with explicit reason
- **Coverage of meaningful cells**: ~88% (84 of 96 = pinned + scope-out)

## 5. Open questions surfaced by the matrix

The following landed in the spec doc (§14 / §16) as numbered open questions. They are *not* defects; they are points where the current implementation makes a choice that the spec leaves implicit, and where the test cell pins the *current* choice (= characterization test) rather than a *correct* answer.

- **OQ-19** `--strict --explain` priority. Current: `--explain` wins (exit 0 even with stale + lit-miss). Should `--strict` raise the exit code in diagnostic mode?
- **OQ-20** `--explain` prints `[missing, will fail]` while exit is 0. The string promises failure that won't happen this invocation. Wording vs exit code mismatch.
- **OQ-21** `--strict` short-circuits the run when *any* pair has a literal miss, even if other pairs have stale rows that the user would also want to see. Should `--strict` aggregate?
- **OQ-22** Empty TO string (`""`) silently becomes `.` via `path.Clean`, then matches against cwd dir. Same class of silent-green hole as DR-0028 blocker #3 (= literal FROM typo). Should empty TO be a usage error? *(openConcern: correctness gap, not just style)*
- **OQ-23** Pair-N syntax error short-circuits earlier pairs — even though pair 1 might be the user's main concern. Cross-pair errors silently lose pair 1's stale rows. Should pairs accumulate errors and report all?
- **OQ-24** Cross-source auto-exclude is not implemented (§6 explicit "undefined"). The current behavior keeps the cross row (= source A's derived path == source B itself is reported as fresh against B's ts). Pinned for v0.2 spec-finalization comparison.
- **OQ-25 (bug, openConcern)** `--glob-ignorecase` causes a grammar-drift panic whenever a matched path's case differs from the FROM pattern's. `expandGlob` passes `WithCaseInsensitive()` to doublestar (fs walk goes case-insensitive), but `buildRawAndRegex` in `glob_backref.go` compiles the capture regex case-sensitively. Any case-different match → doublestar match × capture-regex no-match → §3.3 panic. K21 pins the panic as characterization; the fix is to thread the IgnoreCase flag into the capture-regex compile (`(?i)` prefix or per-group). Not blocking v0.31.0 since `--glob-ignorecase` was never advertised for `vcs outdated`, but a v0.32.0 follow-up. Reproduced on darwin/APFS: pattern `**/*.md` with `DOCS/README.MD` on disk + `--glob-ignorecase`.

## 6. References

- Spec: `docs/specs/glob-backref-v0.1.0.md`
- DR: `docs/decisions/DR-0028-glob-backref-spec-v0.1.0-adoption.md`, `docs/decisions/DR-0027-derived-sync-mini-dsl-and-regex-reject.md`
- v0.31.0 release commit: `adca7472`

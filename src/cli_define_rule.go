package main

import "fmt"

// cli_define_rule.go owns the data model for DR-0029 user-defined rule
// blocks. The CLI flag parser populates these structures; the rule
// dispatcher consumes them to override builtin behaviour on a per-SOURCE
// basis. See docs/decisions/DR-0029-cli-user-defined-rule-phase1.md.
//
// Design rationale: --define-rule blocks deliberately make
// --format/--version-*/--name-* order-dependent (scope-sensitive). This
// is the only flag family in bump-semver with positional flag semantics
// — every other flag is order-independent. The trade-off is justified
// in DR-0029 § "Negative" because:
//
//   - block 内の flag 順序自体は不問 (= ブロック宣言の枠内で
//     order-independent)。
//   - "block の境界" だけが --define-rule の位置で決まる (= block 開始
//     のみ position-sensitive)。
//   - 表現力 (= 1 invocation で複数 SOURCE に別 rule を割り当てる) が
//     position-sensitivity を含めても他案より優位。
//
// 違反検出: 0a 補強 ("最初の --define-rule 出現後に書く rule 系 flag は
// 必ずいずれかのブロックに属さねばならない、ブロックを跨ぐ位置の rule 系
// flag は error") は parser で適用する。argparse layer の unknown option
// 検出と組み合わせて typo 防御を実現する。

// ruleOpts groups the five rule-definition flags. Each is *string so
// "unset" (nil) is structurally distinguishable from "explicitly set to
// empty" (non-nil &""). Parser rejects empty values for required slots
// so callers can treat non-nil pointers as non-empty.
//
// Format selects parser/serializer (DR-0029 § "Phase 1 で必要な flag"):
//
//	"text" → parser-less, VersionRegex required, VersionPath unusable
//	"json" / "yaml" / "toml" / "xml" → tree-parsed, VersionPath primary,
//	                           VersionRegex optional (2-stage extract
//	                           on path-value or whole-file regex)
//
// "xml" uses the SAME dot-path language as json/yaml/toml (DR-0029 §
// "パス言語統一"). XML's structural difference (a node carries both
// child elements and attributes) is resolved by checking both
// interpretations of the final path segment: exactly one match wins,
// both-equal is accepted, both-different is an ambiguous error (see
// format_xml_dotpath.go). textContent is trimmed on read and rewritten
// in place (surrounding whitespace preserved). The codex C-3 concern
// (XML tree differs) is met by this dual-resolution rule rather than a
// separate path language.
type ruleOpts struct {
	Format       *string
	VersionPath  *string
	VersionRegex *string
	NamePath     *string
	NameRegex    *string
}

// hasAny reports whether at least one rule-definition flag was set.
// Used to detect "empty block" (= --define-rule X --define-rule Y with
// no rule flag in the X block — see DR-0029 § "Flag のスコープ規約
// 評価規則 4": empty block is error).
func (o ruleOpts) hasAny() bool {
	return o.Format != nil || o.VersionPath != nil || o.VersionRegex != nil ||
		o.NamePath != nil || o.NameRegex != nil
}

// ruleBlock is one --define-rule scope. The zero-th block represents
// the global scope (Pattern == ""), populated from rule-definition
// flags appearing before any --define-rule. Subsequent blocks are
// opened by --define-rule PATTERN and carry the PATTERN literal as
// supplied by the user (no normalisation at parse time — that happens
// in the rule dispatcher together with each SOURCE).
type ruleBlock struct {
	Pattern string // "" for the global block, otherwise the --define-rule arg
	Opts    ruleOpts
}

// isGlobal reports whether this block is the implicit global scope.
func (b ruleBlock) isGlobal() bool { return b.Pattern == "" }

// allowedRuleFormats enumerates valid --format values for DR-0029
// user-defined rules. xml uses the unified dot-path language (see
// format_xml_dotpath.go). The internal-only builtin formats
// (xml-element / pbxproj / plist-flavoured xml) are NOT exposed to
// --define-rule users — CLI xml goes through xmlDot* exclusively.
var allowedRuleFormats = map[string]bool{
	"text": true, "json": true, "yaml": true, "toml": true, "xml": true,
}

// ensureRuleBlocks lazy-initialises out.ruleBlocks with the implicit
// global block. Called from the parser when the first rule-definition
// flag (or --define-rule) appears, so invocations that never use
// --define-rule keep ruleBlocks == nil and look byte-identical to the
// pre-DR-0029 cliArgs shape (= existing parse tests don't need to
// update their want values).
func ensureRuleBlocks(out *cliArgs) {
	if out.ruleBlocks == nil {
		out.ruleBlocks = []ruleBlock{{Pattern: ""}}
	}
}

// assignRuleFlag writes the given rule-definition flag value into the
// currently-active rule block, enforcing four rules from DR-0029:
//
//  1. 値が空文字 → error (= "use --no-foo to remove" style; rule-
//     definition flags have no "remove" form, an empty value is
//     always wrong).
//  2. 0a 補強: 最初の --define-rule 出現後は global block (ruleBlocks[0])
//     に書き込めない。違反は "must come before any --define-rule" の
//     hint 付き error。
//  3. 0c: 同じ block 内に同じ flag を 2 回書くと error (= last-write-
//     wins より surprise が少ない、意図不明)。
//  4. --format 専用: text|json|yaml|toml|xml 以外は error。
//
// targetField is one of "Format" / "VersionPath" / "VersionRegex" /
// "NamePath" / "NameRegex"; assignRuleFlag uses it to pick the right
// pointer slot on ruleOpts.
func assignRuleFlag(out *cliArgs, flagName, targetField, value string) error {
	if value == "" {
		return fmt.Errorf("%s value cannot be empty", flagName)
	}
	if targetField == "Format" && !allowedRuleFormats[value] {
		return fmt.Errorf("%s value %q is not a valid format (expected one of text|json|yaml|toml|xml)", flagName, value)
	}
	ensureRuleBlocks(out)
	// 0a 補強: once --define-rule has appeared, the global block is
	// closed for new writes. Any subsequent rule flag must follow a
	// --define-rule (= belong to a named block). The most recent block
	// is therefore always at the tail of ruleBlocks; if that tail is
	// the global block (index 0) AND hasDefineRule is set, the user
	// mis-positioned the flag.
	last := len(out.ruleBlocks) - 1
	if out.hasDefineRule && out.ruleBlocks[last].isGlobal() {
		// This should be impossible: once hasDefineRule flips to true,
		// at least one named block has been appended. Defence in depth.
		return fmt.Errorf("%s appeared after --define-rule but outside any block; rule-definition flags must follow a --define-rule and stay within its block until the next --define-rule (or the end of argv). To set a global default, place the flag BEFORE any --define-rule", flagName)
	}
	opts := &out.ruleBlocks[last].Opts
	switch targetField {
	case "Format":
		if opts.Format != nil {
			return ruleBlockDupErr(flagName, out.ruleBlocks[last])
		}
		v := value
		opts.Format = &v
	case "VersionPath":
		if opts.VersionPath != nil {
			return ruleBlockDupErr(flagName, out.ruleBlocks[last])
		}
		v := value
		opts.VersionPath = &v
	case "VersionRegex":
		if opts.VersionRegex != nil {
			return ruleBlockDupErr(flagName, out.ruleBlocks[last])
		}
		v := value
		opts.VersionRegex = &v
	case "NamePath":
		if opts.NamePath != nil {
			return ruleBlockDupErr(flagName, out.ruleBlocks[last])
		}
		v := value
		opts.NamePath = &v
	case "NameRegex":
		if opts.NameRegex != nil {
			return ruleBlockDupErr(flagName, out.ruleBlocks[last])
		}
		v := value
		opts.NameRegex = &v
	default:
		// Programmer error — caller passed an unknown field name.
		return fmt.Errorf("internal: assignRuleFlag got unknown targetField %q", targetField)
	}
	return nil
}

// ruleBlockDupErr renders the 0c "same flag twice in one block" error
// with the block's PATTERN (or "global" label) for diagnosis.
func ruleBlockDupErr(flagName string, b ruleBlock) error {
	if b.isGlobal() {
		return fmt.Errorf("%s specified twice in the global rule block", flagName)
	}
	return fmt.Errorf("%s specified twice in --define-rule %q block", flagName, b.Pattern)
}

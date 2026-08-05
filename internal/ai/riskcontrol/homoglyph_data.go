package riskcontrol

// builtinHomoglyphs maps characters commonly used to bypass keyword filters
// to their normalized equivalents. NFKC already handles full-width →
// half-width and compatibility decompositions, so this table only covers
// characters that NFKC leaves untouched but which are visually or
// semantically equivalent in a bypass context (e.g. look-alike symbols,
// enclosed alphanumerics, mathematical symbols).
//
// Site-specific overrides can be added via KeywordConfig.HomoglyphMapExtra
// and are merged on top of this table at compile time.
var builtinHomoglyphs = map[rune]rune{
	// Enclosed / circled alphanumerics (NFKC handles some but not all)
	'⓪': '0', '①': '1', '②': '2', '③': '3', '④': '4',
	'⑤': '5', '⑥': '6', '⑦': '7', '⑧': '8', '⑨': '9',
	'⓵': '1', '⓶': '2', '⓷': '3', '⓸': '4', '⓹': '5',
	'⓺': '6', '⓻': '7', '⓼': '8', '⓽': '9', '⓿': '0',

	// Superscripts / subscripts
	'⁰': '0', '¹': '1', '²': '2', '³': '3', '⁴': '4',
	'⁵': '5', '⁶': '6', '⁷': '7', '⁸': '8', '⁹': '9',
	'₀': '0', '₁': '1', '₂': '2', '₃': '3', '₄': '4',
	'₅': '5', '₆': '6', '₇': '7', '₈': '8', '₉': '9',

	// Roman numerals (common in bypass attempts)
	'Ⅰ': 'I', 'Ⅱ': 'I', 'Ⅲ': 'I', 'Ⅳ': 'I', 'Ⅴ': 'V',
	'Ⅵ': 'V', 'Ⅶ': 'V', 'Ⅷ': 'V', 'Ⅸ': 'I', 'Ⅹ': 'X',

	// Look-alike Latin → ASCII (Cyrillic/Greek confusables)
	'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c',
	'у': 'y', 'х': 'x', 'А': 'A', 'В': 'B', 'Е': 'E',
	'К': 'K', 'М': 'M', 'Н': 'H', 'О': 'O', 'Р': 'P',
	'С': 'C', 'Т': 'T', 'У': 'Y', 'Х': 'X',
	'α': 'a', 'β': 'b', 'γ': 'g', 'δ': 'd', 'ε': 'e',
	'θ': 'o', 'ι': 'i', 'κ': 'k', 'ν': 'v', 'ο': 'o',
	'ρ': 'p', 'σ': 's', 'τ': 't', 'υ': 'u', 'ω': 'w',

	// Zero-width and invisible characters (stripped entirely → mapped to 0-width)
	'\u200B': 0, // zero-width space
	'\u200C': 0, // zero-width non-joiner
	'\u200D': 0, // zero-width joiner
	'\uFEFF': 0, // zero-width no-break space (BOM)

	// Special dot/symbol substitutions used in separator bypass
	'·': '·', '・': '·', '•': '·', '●': '·', '○': '·',
	'᛫': '·', '‧': '·', '⁞': '·', '∣': '|',
}

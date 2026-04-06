package common

import "go/token"

// ArithmeticFlipPairs maps arithmetic operators to their flip counterparts:
// + ↔ -, * ↔ /
var ArithmeticFlipPairs = map[token.Token]token.Token{
	token.ADD: token.SUB,
	token.SUB: token.ADD,
	token.MUL: token.QUO,
	token.QUO: token.MUL,
}

// ArithmeticFlipTokens is the set of tokens this mutation applies to (for CanApply checks).
var ArithmeticFlipTokens = map[token.Token]struct{}{
	token.ADD: {},
	token.SUB: {},
	token.MUL: {},
	token.QUO: {},
}

// ComparisonNegationPairs maps comparison operators to their negated forms:
// == ↔ !=, < ↔ >=, > ↔ <=
var ComparisonNegationPairs = map[token.Token]token.Token{
	token.EQL: token.NEQ,
	token.NEQ: token.EQL,
	token.LSS: token.GEQ,
	token.LEQ: token.GTR,
	token.GTR: token.LEQ,
	token.GEQ: token.LSS,
}

// ComparisonNegationTokens is the set of tokens this mutation applies to.
var ComparisonNegationTokens = map[token.Token]struct{}{
	token.EQL: {},
	token.NEQ: {},
	token.LSS: {},
	token.LEQ: {},
	token.GTR: {},
	token.GEQ: {},
}

// BoundaryValuePairs maps boundary operators to their boundary variants:
// < ↔ <=, > ↔ >=
var BoundaryValuePairs = map[token.Token]token.Token{
	token.LSS: token.LEQ,
	token.LEQ: token.LSS,
	token.GTR: token.GEQ,
	token.GEQ: token.GTR,
}

// BoundaryValueTokens is the set of tokens this mutation applies to.
var BoundaryValueTokens = map[token.Token]struct{}{
	token.LSS: {},
	token.GTR: {},
	token.LEQ: {},
	token.GEQ: {},
}

// LogicalOperatorPairs maps logical operators to their opposites:
// && ↔ ||
var LogicalOperatorPairs = map[token.Token]token.Token{
	token.LAND: token.LOR,
	token.LOR:  token.LAND,
}

// LogicalOperatorTokens is the set of tokens this mutation applies to.
var LogicalOperatorTokens = map[token.Token]struct{}{
	token.LAND: {},
	token.LOR:  {},
}

// BinaryMathPairs maps bitwise/shift operators to their counterparts:
// % → *, & ↔ |, << ↔ >>
var BinaryMathPairs = map[token.Token]token.Token{
	token.REM: token.MUL,
	token.AND: token.OR,
	token.OR:  token.AND,
	token.SHL: token.SHR,
	token.SHR: token.SHL,
}

// BinaryMathTokens is the set of tokens this mutation applies to.
var BinaryMathTokens = map[token.Token]struct{}{
	token.REM: {},
	token.AND: {},
	token.OR:  {},
	token.SHL: {},
	token.SHR: {},
}

// IncDecPairs maps increment/decrement operators to their opposites:
// ++ ↔ --
var IncDecPairs = map[token.Token]token.Token{
	token.INC: token.DEC,
	token.DEC: token.INC,
}

// IncDecTokens is the set of tokens this mutation applies to.
var IncDecTokens = map[token.Token]struct{}{
	token.INC: {},
	token.DEC: {},
}

// SignTogglePairs maps unary sign operators to their opposites:
// +x ↔ -x
var SignTogglePairs = map[token.Token]token.Token{
	token.SUB: token.ADD,
	token.ADD: token.SUB,
}

// SignToggleTokens is the set of tokens this mutation applies to.
var SignToggleTokens = map[token.Token]struct{}{
	token.SUB: {},
	token.ADD: {},
}

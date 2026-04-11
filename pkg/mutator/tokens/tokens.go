// Package tokens defines operator flip maps used by token-based mutation operators.
package tokens

import "go/token"

var ArithmeticFlipPairs = map[token.Token]token.Token{
	token.ADD: token.SUB,
	token.SUB: token.ADD,
	token.MUL: token.QUO,
	token.QUO: token.MUL,
}

var ArithmeticFlipTokens = map[token.Token]struct{}{
	token.ADD: {},
	token.SUB: {},
	token.MUL: {},
	token.QUO: {},
}

var ComparisonNegationPairs = map[token.Token]token.Token{
	token.EQL: token.NEQ,
	token.NEQ: token.EQL,
	token.LSS: token.GEQ,
	token.LEQ: token.GTR,
	token.GTR: token.LEQ,
	token.GEQ: token.LSS,
}

var ComparisonNegationTokens = map[token.Token]struct{}{
	token.EQL: {},
	token.NEQ: {},
	token.LSS: {},
	token.LEQ: {},
	token.GTR: {},
	token.GEQ: {},
}

var BoundaryValuePairs = map[token.Token]token.Token{
	token.LSS: token.LEQ,
	token.LEQ: token.LSS,
	token.GTR: token.GEQ,
	token.GEQ: token.GTR,
}

var BoundaryValueTokens = map[token.Token]struct{}{
	token.LSS: {},
	token.GTR: {},
	token.LEQ: {},
	token.GEQ: {},
}

var LogicalOperatorPairs = map[token.Token]token.Token{
	token.LAND: token.LOR,
	token.LOR:  token.LAND,
}

var LogicalOperatorTokens = map[token.Token]struct{}{
	token.LAND: {},
	token.LOR:  {},
}

var BinaryMathPairs = map[token.Token]token.Token{
	token.REM: token.MUL,
	token.AND: token.OR,
	token.OR:  token.AND,
	token.SHL: token.SHR,
	token.SHR: token.SHL,
}

var BinaryMathTokens = map[token.Token]struct{}{
	token.REM: {},
	token.AND: {},
	token.OR:  {},
	token.SHL: {},
	token.SHR: {},
}

var IncDecPairs = map[token.Token]token.Token{
	token.INC: token.DEC,
	token.DEC: token.INC,
}

var IncDecTokens = map[token.Token]struct{}{
	token.INC: {},
	token.DEC: {},
}

var SignTogglePairs = map[token.Token]token.Token{
	token.SUB: token.ADD,
	token.ADD: token.SUB,
}

var SignToggleTokens = map[token.Token]struct{}{
	token.SUB: {},
	token.ADD: {},
}

func SwapBinaryToken(op token.Token, pairs map[token.Token]token.Token) (token.Token, bool) {
	newOp, ok := pairs[op]
	return newOp, ok
}

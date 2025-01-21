// Package token defines constants representing the lexical tokens.
package token

type Token int

const (
	ILLEGAL Token = iota
	EOF

	STRING // "text"
	INT    // 123
	FLOAT  // 1.23
	BOOL   // true
	IDENT  // vars

	ADD  // +
	SUB  // -
	MUL  // *
	QUO  // /
	REM  // %
	CALL // }}:\n

	LAND // &&
	LOR  // ||

	EQL // ==
	NEQ // !=
	LSS // <
	LEQ // <=
	GTR // >
	GEQ // >=
	NOT // !

	LPAREN    // (
	RPAREN    // )
	LBRACK    // [
	RBRACK    // ]
	LDBRACE   // {{
	RDBRACE   // }}
	COMMA     // ,
	PERIOD    // .
	QUESTION  // ?
	COLON     // :
	LARROW    // <-
	CONCAT    // implicit concatenation
	LINEBREAK // end of a larrow expression argument

	DEFINED  // defined
	COALESCE // coalesce
)

// String returns t as string.
func (t Token) String() string {
	switch t {
	case ILLEGAL:
		return "illegal"
	case EOF:
		return "EOF"
	case STRING:
		return "string"
	case INT:
		return "int"
	case FLOAT:
		return "float"
	case BOOL:
		return "bool"
	case IDENT:
		return "ident"
	case ADD:
		return "+"
	case SUB:
		return "-"
	case MUL:
		return "*"
	case QUO:
		return "/"
	case REM:
		return "%"
	case CALL:
		return "call"
	case LAND:
		return "&&"
	case LOR:
		return "||"
	case EQL:
		return "=="
	case NEQ:
		return "!="
	case LSS:
		return "<"
	case LEQ:
		return "<="
	case GTR:
		return ">"
	case GEQ:
		return ">="
	case NOT:
		return "!"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACK:
		return "["
	case RBRACK:
		return "]"
	case LDBRACE:
		return "{{"
	case RDBRACE:
		return "}}"
	case COMMA:
		return ","
	case PERIOD:
		return "."
	case QUESTION:
		return "?"
	case COLON:
		return ":"
	case LARROW:
		return "<-"
	case CONCAT:
		return "implicitly concatenate"
	case LINEBREAK:
		return "line break"
	case DEFINED:
		return "defined"
	case COALESCE:
		return "coalesce"
	}
	return "unknown"
}

// A set of constants for precedence-based expression parsing.
// Non-operators have lowest precedence.
const (
	LowestPrec  = 0 // non-operators
	HighestPrec = 7
)

// Precedence returns the operator precedence of the binary
// operator op. If op is not a binary operator, the result
// is LowestPrecedence.
func (t Token) Precedence() int {
	switch t {
	case QUESTION, COLON:
		return 1
	case LOR:
		return 2
	case LAND:
		return 3
	case EQL, NEQ, LSS, LEQ, GTR, GEQ:
		return 4
	case ADD, SUB, LARROW, LDBRACE, STRING:
		return 5
	case MUL, QUO, REM:
		return 6
	default:
		return LowestPrec
	}
}

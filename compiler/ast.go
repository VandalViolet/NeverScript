package compiler

type AstNode struct {
	Kind AstKind
	Data AstData
}

type AstKind int

const (
	AstKind_Root = iota
	AstKind_Assignment
	AstKind_Invocation
	AstKind_Comment
	AstKind_NewLine
	AstKind_Script
	AstKind_WhileLoop
	AstKind_Break
	AstKind_Return
	AstKind_IfStatement
	AstKind_LogicalNot
	AstKind_LogicalAnd
	AstKind_LogicalOr
	AstKind_LocalReference
	AstKind_AllArguments
	AstKind_Checksum
	AstKind_Float
	AstKind_Integer
	AstKind_String
	AstKind_AdditionExpression
	AstKind_SubtractionExpression
	AstKind_MultiplicationExpression
	AstKind_DivisionExpression
	AstKind_GreaterThanExpression
	AstKind_GreaterThanEqualsExpression
	AstKind_LessThanExpression
	AstKind_LessThanEqualsExpression
	AstKind_EqualsExpression
	AstKind_NotEqualExpression
	AstKind_DotExpression
	AstKind_ColonExpression
	AstKind_UnaryExpression
	AstKind_Pair
	AstKind_Vector
	AstKind_Struct
	AstKind_Array
	AstKind_ArrayAccess
	AstKind_Comma
	AstKind_Random
	AstKind_RandomRange
	AstKind_EndOfFile
	AstKind_NameTableEntry
	AstKind_FlatExpression
)

func (astKind AstKind) String() string {
	return [...]string{
		"AstKind_Root",
		"AstKind_Assignment",
		"AstKind_Invocation",
		"AstKind_Comment",
		"AstKind_NewLine",
		"AstKind_Script",
		"AstKind_WhileLoop",
		"AstKind_Break",
		"AstKind_Return",
		"AstKind_IfStatement",
		"AstKind_LogicalNot",
		"AstKind_LogicalAnd",
		"AstKind_LogicalOr",
		"AstKind_LocalReference",
		"AstKind_AllArguments",
		"AstKind_Checksum",
		"AstKind_Float",
		"AstKind_Integer",
		"AstKind_String",
		"AstKind_AdditionExpression",
		"AstKind_SubtractionExpression",
		"AstKind_MultiplicationExpression",
		"AstKind_DivisionExpression",
		"AstKind_GreaterThanExpression",
		"AstKind_GreaterThanEqualsExpression",
		"AstKind_LessThanExpression",
		"AstKind_LessThanEqualsExpression",
		"AstKind_EqualsExpression",
		"AstKind_NotEqualExpression",
		"AstKind_DotExpression",
		"AstKind_ColonExpression",
		"AstKind_UnaryExpression",
		"AstKind_Pair",
		"AstKind_Vector",
		"AstKind_Struct",
		"AstKind_Array",
		"AstKind_ArrayAccess",
		"AstKind_Comma",
		"AstKind_Random",
		"AstKind_RandomRange",
		"AstKind_EndOfFile",
		"AstKind_NameTableEntry",
		"AstKind_FlatExpression",
	}[astKind]
}

type AstData interface {
	astData()
}

type AstData_Root struct {
	BodyNodes []AstNode
}
func (astData AstData_Root) astData() {}

type AstData_Assignment struct {
	NameNode  AstNode
	ValueNode AstNode
}
func (astData AstData_Assignment) astData() {}

type AstData_Invocation struct {
	ScriptIdentifierNode              AstNode
	ParameterNodes                    []AstNode
	TokensConsumedByEachParameterNode []int
}
func (astData AstData_Invocation) astData() {}

type AstData_Empty struct{}
func (astData AstData_Empty) astData() {}

type AstData_Script struct {
	NameNode              AstNode
	DefaultParameterNodes []AstNode
	BodyNodes             []AstNode
}
func (astData AstData_Script) astData() {}

type AstData_WhileLoop struct {
	BodyNodes    []AstNode
}
func (astData AstData_WhileLoop) astData() {}

type AstData_IfStatement struct {
	Conditions            []AstNode
	Bodies                [][]AstNode
}
func (astData AstData_IfStatement) astData() {}

type AstData_Comment struct {
	CommentToken Token
}
func (astData AstData_Comment) astData() {}

type AstData_LocalReference struct {
	Node AstNode
}
func (astData AstData_LocalReference) astData() {}

type AstData_Checksum struct {
	IsRawChecksum bool
	ChecksumToken Token
	ChecksumBytes []byte
}
func (astData AstData_Checksum) astData() {}

type AstData_Float struct {
	FloatToken Token
	FloatBytes []byte
}
func (astData AstData_Float) astData() {}

type AstData_Integer struct {
	IntegerToken Token
	IntegerBytes []byte
}
func (astData AstData_Integer) astData() {}

type AstData_String struct {
	StringToken Token
	StringBytes []byte
}
func (astData AstData_String) astData() {}

type AstData_BinaryExpression struct {
	LeftNode  AstNode
	RightNode AstNode
}
func (astData AstData_BinaryExpression) astData() {}

// AstData_FlatExpression represents a parenthesised expression containing two or
// more operators, e.g. (A = 0 or B = <c>). THUG2 stores these as a single flat
// infix token stream between one 0xE/0xF pair (operands and operator bytes
// inline, with no nested parentheses), so we keep the operands and operator
// kinds as ordered lists and emit them verbatim. len(Operators) == len(Operands)-1.
type AstData_FlatExpression struct {
	Operands  []AstNode
	Operators []AstKind
}
func (astData AstData_FlatExpression) astData() {}

type AstData_Pair struct {
	FloatNodeA AstNode
	FloatNodeB AstNode
}
func (astData AstData_Pair) astData() {}

type AstData_Vector struct {
	FloatNodeA AstNode
	FloatNodeB AstNode
	FloatNodeC AstNode
}
func (astData AstData_Vector) astData() {}

type AstData_UnaryExpression struct {
	Node AstNode
}
func (astData AstData_UnaryExpression) astData() {}

type AstData_Struct struct {
	ElementNodes []AstNode
}
func (astData AstData_Struct) astData() {}

type AstData_Array struct {
	ElementNodes []AstNode
}
func (astData AstData_Array) astData() {}

type AstData_ArrayAccess struct {
	Array AstNode
	Index AstNode
}
func (astData AstData_ArrayAccess) astData() {}


type AstData_Random struct {
	BranchWeights []AstNode
	Branches [][]AstNode
	IsNoRepeat bool // true => emit 0x40 (random2/no-repeat) instead of 0x2F
	// Branch0Newline: does the original have a 0x01 between the offset table and
	// the first branch? It's per-random (formatting-driven, not type-driven), so we
	// preserve it via a newline right after `{` instead of force-emitting it. Needed
	// for byte-identity; the offset table's +1 base depends on it.
	Branch0Newline bool
}
func (astData AstData_Random) astData() {}

// AstData_NameTableEntry carries orphan checksum names — symbols that the
// original .qb registered in its trailing 0x2b name table but never referenced
// in code (e.g. `printf`). The decompiler emits them via a top-level
// `__register_checksums__ <names...>` directive so the compiler re-adds them to
// the name table (no body bytecode). Without this, the recompiled table drops
// them, which e.g. blanks the on-screen combo score (printf formats it).
type AstData_NameTableEntry struct {
	Names []string
}
func (astData AstData_NameTableEntry) astData() {}
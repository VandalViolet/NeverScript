package compiler

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type CustomByteBuffer struct {
	Buffer []byte
}

type BytecodeCompiler struct {
	RootAstNode        AstNode
	Bytes              []byte
	NextLoopBypasserId int
	TargetGame         string
	RemoveChecksums    bool
}

func GenerateBytecode(compiler *BytecodeCompiler) {
	write := func(bytes ...byte) {
		compiler.Bytes = append(compiler.Bytes, bytes...)
	}

	writeIndex := func(index int, bytes ...byte) {
		i := index
		j := 0
		for {
			if i >= len(compiler.Bytes) || j >= len(bytes) {
				break
			}
			compiler.Bytes[i] = bytes[j]
			i++
			j++
		}
	}

	writeLittleUint32 := func(n uint32) {
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, n)
		write(bytes...)
	}

	writeLittleUint32Index := func(n uint32, index int) {
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, n)
		writeIndex(index, bytes...)
	}

	writeLittleUint16 := func(n uint16) {
		bytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(bytes, n)
		write(bytes...)
	}

	writeLittleUint16Index := func(n uint16, index int) {
		bytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(bytes, n)
		writeIndex(index, bytes...)
	}

	nameTable := make(map[string]uint32)

	// Explicit trailing-name-table order, populated by __register_checksums__
	// directives (which the decompiler emits to pin the table to the original's
	// order — THUG2's loader requires it). When non-empty this overrides the
	// map-iteration order below.
	var nameTableOrder []string
	nameTableOrderSeen := make(map[string]bool)

	var writeBytecodeForNode func(node AstNode)
	var writeBytecodeForIf func(node AstNode)
	var writeBytecodeForIfElse func(conditionNode AstNode, bodyNodes []AstNode, elseNodes []AstNode, hasElse bool)
	var writeBytecodeForBinaryExpression func(node AstNode, operator byte)
	var writeBytecodeForBinaryExpressionWithParentheses func(node AstNode, operator byte)
	var writeBytecodeForChecksum func(node AstNode)
	var writeBytecodeForPair func(node AstNode)
	var writeBytecodeForVector func(node AstNode)
	var writeBytecodeForInteger func(node AstNode)
	var writeBytecodeForFloat func(node AstNode)

	writeBytecodeForNode = func(node AstNode) {
		switch node.Kind {
		case AstKind_Root:
			for _, rootNode := range node.Data.(AstData_Root).BodyNodes {
				writeBytecodeForNode(rootNode)
			}
		case AstKind_NewLine:
			write(1)
		case AstKind_Comma:
			write(9)
		case AstKind_Break:
			write(0x22)
		case AstKind_AllArguments:
			write(0x2C)
		case AstKind_LocalReference:
			write(0x2D)
			writeBytecodeForNode(node.Data.(AstData_LocalReference).Node)
		case AstKind_Checksum:
			writeBytecodeForChecksum(node)
		case AstKind_Integer:
			writeBytecodeForInteger(node)
		case AstKind_Float:
			writeBytecodeForFloat(node)
		case AstKind_String:
			write(0x1B)
			stringData := node.Data.(AstData_String).StringToken.Data
			stringData = stringData[1 : len(stringData)-1]
			stringData = strings.Replace(stringData, "\\\\", "\\",-1)
			stringData = strings.Replace(stringData, "\\\"", "\"",-1)
			writeLittleUint32(uint32(len(stringData) + 1))
			write([]byte(stringData)...)
			write(0)
		case AstKind_Pair:
			writeBytecodeForPair(node)
		case AstKind_RandomRange:
			// 0x30 followed by a pair (0x1F + two floats)
			write(0x30)
			writeBytecodeForNode(node.Data.(AstData_UnaryExpression).Node)
		case AstKind_Vector:
			writeBytecodeForVector(node)
		case AstKind_UnaryExpression:
			write(0xE)
			writeBytecodeForNode(node.Data.(AstData_UnaryExpression).Node)
			write(0xF)
		case AstKind_SubtractionExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0xA)
		case AstKind_AdditionExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0xB)
		case AstKind_DivisionExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0xC)
		case AstKind_MultiplicationExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0xD)
		case AstKind_LessThanExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0x12)
		case AstKind_LessThanEqualsExpression:
			writeBytecodeForNode(AstNode{
				Kind: AstKind_LogicalNot,
				Data: AstData_UnaryExpression{
					Node: AstNode{
						Kind: AstKind_GreaterThanExpression,
						Data: node.Data,
					},
				},
			})
		case AstKind_GreaterThanExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0x14)
		case AstKind_GreaterThanEqualsExpression:
			writeBytecodeForNode(AstNode{
				Kind: AstKind_LogicalNot,
				Data: AstData_UnaryExpression{
					Node: AstNode{
						Kind: AstKind_LessThanExpression,
						Data: node.Data,
					},
				},
			})
		case AstKind_EqualsExpression:
			writeBytecodeForBinaryExpressionWithParentheses(node, 0x7)
		case AstKind_NotEqualExpression:
			writeBytecodeForNode(AstNode{
				Kind: AstKind_LogicalNot,
				Data: AstData_UnaryExpression{
					Node: AstNode{
						Kind: AstKind_EqualsExpression,
						Data: node.Data,
					},
				},
			})
		case AstKind_DotExpression:
			writeBytecodeForBinaryExpression(node, 0x8)
		case AstKind_ColonExpression:
			writeBytecodeForBinaryExpression(node, 0x42)
		case AstKind_LogicalNot:
			write(0x39)
			writeBytecodeForNode(node.Data.(AstData_UnaryExpression).Node)
		case AstKind_LogicalAnd:
			writeBytecodeForBinaryExpression(node, 0x33)
		case AstKind_LogicalOr:
			writeBytecodeForBinaryExpression(node, 0x32)
		case AstKind_Comment:
			//writeBytecodeForNode(AstNode{
			//	Kind: AstKind_String,
			//	Data: AstData_String{
			//		StringToken: Token{
			//			Kind: TokenKind_String,
			//			Data: "\"" + node.Data.(AstData_Comment).CommentToken.Data + "\"",
			//		},
			//	},
			//})
		case AstKind_Script:
			data := node.Data.(AstData_Script)
			write(0x23)
			writeBytecodeForNode(data.NameNode)
			for _, defaultParameterNode := range data.DefaultParameterNodes {
				writeBytecodeForNode(defaultParameterNode)
			}
			for _, bodyNode := range data.BodyNodes {
				writeBytecodeForNode(bodyNode)
			}
			write(0x24)
		case AstKind_IfStatement:
			writeBytecodeForIf(node)
		case AstKind_Random:
			data := node.Data.(AstData_Random)

			numBranches := len(data.Branches)

			if data.IsNoRepeat {
				write(0x40)
			} else {
				write(0x2F)
			}
			writeLittleUint32(uint32(numBranches))

			// write branch weights
			for i := 0; i < numBranches; i++ {
				branchWeightAsInt, _ := strconv.ParseInt(data.BranchWeights[i].Data.(AstData_Integer).IntegerToken.Data, 10, 32)
				writeLittleUint16(uint16(branchWeightAsInt))
			}

			branchOffsetsIndex := len(compiler.Bytes)

			// write dummy branch offsets (populate later)
			for i := 0; i < numBranches; i++ {
				var offset uint32 = 2
				writeLittleUint32(offset)
			}

			// Most randoms place a single newline (0x01) between the offset table
			// and the first branch, and every branch offset is measured to account
			// for it (+1 below). But it's per-random (formatting), so emit it only
			// when the source had it (a newline right after `{`); otherwise the
			// offsets must NOT include the +1.
			if data.Branch0Newline {
				write(0x01)
			}

			// write branches (record sizes for offset calculations, record longjump positions)
			branchSizes := make([]int, numBranches)
			longJumpPositions := make([]int, numBranches-1)
			for i := 0; i < numBranches; i++ {
				start := len(compiler.Bytes)
				for _, branchNode := range data.Branches[i] {
					writeBytecodeForNode(branchNode)
				}
				if i < (numBranches - 1) {
					// write dummy longjump offset (populate later)
					longJumpPositions[i] = len(compiler.Bytes)
					write(0x2e)
					writeLittleUint32(0)
				}
				end := len(compiler.Bytes)
				branchSizes[i] = end - start
			}

			finalIndex := len(compiler.Bytes)

			// update branch offsets with real values
			for i := 0; i < numBranches; i++ {
				offsetIndex := branchOffsetsIndex + (4 * i)

				offsetValue := 0
				if data.Branch0Newline {
					offsetValue = 1 // +1 for the 0x01 newline before the first branch
				}

				// include next branch offsets in offsetValue
				for j := i + 1; j < numBranches; j++ {
					offsetValue += 4
				}

				// include previous branch sizes in offsetValue too
				for j := 0; j < i; j++ {
					offsetValue += branchSizes[j]
				}

				writeLittleUint32Index(uint32(offsetValue), offsetIndex)
			}

			// update longjump offsets with real values
			for i := 0; i < numBranches-1; i++ {
				realOffset := finalIndex - longJumpPositions[i] - 5
				writeLittleUint32Index(uint32(realOffset), longJumpPositions[i]+1)
			}

		case AstKind_WhileLoop:
			// Emit a plain begin/repeat loop (0x20 ... 0x21), matching the
			// original Neversoft bytecode. (Previously THUG2 builds injected an
			// "infinite_loop_bypasser" guard, but it assigned a GLOBAL while the
			// guard read an undefined LOCAL — semantically wrong and not present
			// in real game scripts, which loop forever via an in-body `wait`.)
			write(0x20)
			for _, bodyNode := range node.Data.(AstData_WhileLoop).BodyNodes {
				writeBytecodeForNode(bodyNode)
			}
			write(0x21)
		case AstKind_Return:
			data := node.Data.(AstData_UnaryExpression)

			var invocationData AstData_Invocation
			if data.Node.Kind == AstKind_Checksum {
				invocationData = AstData_Invocation{
					ScriptIdentifierNode:              data.Node,
					ParameterNodes:                    []AstNode{},
					TokensConsumedByEachParameterNode: []int{},
				}
			} else {
				invocationData = data.Node.Data.(AstData_Invocation)
			}

			write(0x29)
			for _, parameterNode := range invocationData.ParameterNodes {

				// Replace 'true' and 'false' with __boolean_result__ parameter
				if parameterNode.Kind == AstKind_Checksum {
					checksumData := parameterNode.Data.(AstData_Checksum)
					name := checksumData.ChecksumToken.Data
					booleanParameterNode := func(integerToken string) AstNode {
						return AstNode{
							Kind: AstKind_Assignment,
							Data: AstData_Assignment{
								NameNode: AstNode{
									Kind: AstKind_Checksum,
									Data: AstData_Checksum{
										IsRawChecksum: false,
										ChecksumToken: Token{
											Kind: TokenKind_Identifier,
											Data: "__boolean_result__",
										},
									},
								},
								ValueNode: AstNode{
									Kind: AstKind_Integer,
									Data: AstData_Integer{
										IntegerToken: Token{
											Kind: TokenKind_Integer,
											Data: integerToken,
										},
									},
								},
							},
						}
					}
					if name == "true" {
						parameterNode = booleanParameterNode("1")
					} else if name == "false" {
						parameterNode = booleanParameterNode("0")
					}
				}
				writeBytecodeForNode(parameterNode)
			}
		case AstKind_Invocation:
			data := node.Data.(AstData_Invocation)
			writeBytecodeForNode(data.ScriptIdentifierNode)
			for _, parameterNode := range data.ParameterNodes {
				writeBytecodeForNode(parameterNode)
			}
		case AstKind_NameTableEntry:
			// Checksum names declared via __register_checksums__: register them so
			// they appear in the trailing name table, IN THIS ORDER, and emit NO
			// body bytecode.
			for _, name := range node.Data.(AstData_NameTableEntry).Names {
				nameTable[name] = StringToChecksum(name)
				if !nameTableOrderSeen[name] {
					nameTableOrderSeen[name] = true
					nameTableOrder = append(nameTableOrder, name)
				}
			}
		case AstKind_Assignment:
			data := node.Data.(AstData_Assignment)
			writeBytecodeForNode(data.NameNode)
			write(7)
			writeBytecodeForNode(data.ValueNode)
		case AstKind_Struct:
			write(3)
			for _, elementNode := range node.Data.(AstData_Struct).ElementNodes {
				writeBytecodeForNode(elementNode)
			}
			write(4)
		case AstKind_Array:
			write(5)
			for _, elementNode := range node.Data.(AstData_Array).ElementNodes {
				writeBytecodeForNode(elementNode)
			}
			write(6)
		case AstKind_ArrayAccess:
			data := node.Data.(AstData_ArrayAccess)
			writeBytecodeForNode(data.Array)
			write(5)
			writeBytecodeForNode(data.Index)
			write(6)
		default:
			fmt.Printf("Warning: no bytecode generated for AstNode of type '%s'\n", node.Kind.String())
		}
	}

	var transformIfElseIf func(ifElseIfNode AstNode) AstNode
	transformIfElseIf = func(ifElseIfNode AstNode) AstNode {
		ifElseIfData := ifElseIfNode.Data.(AstData_IfStatement)

		if len(ifElseIfData.Conditions) == 1 {
			return ifElseIfNode
		}

		bodies := [][]AstNode{
			ifElseIfData.Bodies[0],
		}

		if len(ifElseIfData.Conditions) > 1 {
			bodies = append(bodies,
				[]AstNode{
					{
						Kind: AstKind_NewLine,
						Data: AstData_Empty{},
					},
					transformIfElseIf(AstNode{
						Kind: AstKind_IfStatement,
						Data: AstData_IfStatement{
							Conditions: ifElseIfData.Conditions[1:],
							Bodies:     ifElseIfData.Bodies[1:],
						},
					}),
					{
						Kind: AstKind_NewLine,
						Data: AstData_Empty{},
					},
				},
			)
		}

		return AstNode{
			Kind: AstKind_IfStatement,
			Data: AstData_IfStatement{
				Conditions: []AstNode{
					ifElseIfData.Conditions[0],
				},
				Bodies: bodies,
			},
		}
	}

	writeBytecodeForIf = func(node AstNode) {
		transformedIf := transformIfElseIf(node)
		transformedIfData := transformedIf.Data.(AstData_IfStatement)
		hasElse := len(transformedIfData.Bodies) > 1
		var elseNodes []AstNode
		if hasElse {
			elseNodes = transformedIfData.Bodies[1]
		}

		writeBytecodeForIfElse(
			transformedIfData.Conditions[0],
			transformedIfData.Bodies[0],
			elseNodes,
			hasElse,
		)
	}

	writeBytecodeForBinaryExpressionWithParentheses = func(node AstNode, operator byte) {
		write(0xE)
		writeBytecodeForBinaryExpression(node, operator)
		write(0xF)
	}

	writeBytecodeForBinaryExpression = func(node AstNode, operator byte) {
		data := node.Data.(AstData_BinaryExpression)
		writeBytecodeForNode(data.LeftNode)
		write(operator)
		writeBytecodeForNode(data.RightNode)
	}

	writeBytecodeForChecksum = func(node AstNode) {
		write(0x16)
		data := node.Data.(AstData_Checksum)

		var checksum uint32
		if data.IsRawChecksum {
			temp1 := data.ChecksumToken.Data[1:]
			temp1 = temp1[6:8] + temp1[4:6] + temp1[2:4] + temp1[0:2]

			temp2, _ := strconv.ParseUint(temp1, 16, 32)
			checksum = uint32(temp2)
		} else {
			name := data.ChecksumToken.Data
			checksum = StringToChecksum(name)
			nameTable[name] = checksum
		}

		writeLittleUint32(checksum)
	}

	writeBytecodeForInteger = func(node AstNode) {
		write(0x17)
		intValue, _ := strconv.ParseInt(node.Data.(AstData_Integer).IntegerToken.Data, 10, 32)
		writeLittleUint32(uint32(intValue))
	}

	writeBytecodeForFloat = func(node AstNode) {
		write(0x1A)
		floatValue, _ := strconv.ParseFloat(node.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValue [4]byte
		binary.LittleEndian.PutUint32(bytesValue[:], math.Float32bits(float32(floatValue)))
		write(bytesValue[:]...)
	}

	writeBytecodeForPair = func(node AstNode) {
		write(0x1F)
		floatValueA, _ := strconv.ParseFloat(node.Data.(AstData_Pair).FloatNodeA.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValueA [4]byte
		binary.LittleEndian.PutUint32(bytesValueA[:], math.Float32bits(float32(floatValueA)))
		write(bytesValueA[:]...)
		floatValueB, _ := strconv.ParseFloat(node.Data.(AstData_Pair).FloatNodeB.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValueB [4]byte
		binary.LittleEndian.PutUint32(bytesValueB[:], math.Float32bits(float32(floatValueB)))
		write(bytesValueB[:]...)
	}

	writeBytecodeForVector = func(node AstNode) {
		write(0x1E)
		floatValueA, _ := strconv.ParseFloat(node.Data.(AstData_Vector).FloatNodeA.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValueA [4]byte
		binary.LittleEndian.PutUint32(bytesValueA[:], math.Float32bits(float32(floatValueA)))
		write(bytesValueA[:]...)
		floatValueB, _ := strconv.ParseFloat(node.Data.(AstData_Vector).FloatNodeB.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValueB [4]byte
		binary.LittleEndian.PutUint32(bytesValueB[:], math.Float32bits(float32(floatValueB)))
		write(bytesValueB[:]...)
		floatValueC, _ := strconv.ParseFloat(node.Data.(AstData_Vector).FloatNodeC.Data.(AstData_Float).FloatToken.Data, 32)
		var bytesValueC [4]byte
		binary.LittleEndian.PutUint32(bytesValueC[:], math.Float32bits(float32(floatValueC)))
		write(bytesValueC[:]...)
	}

	writeBytecodeForIfElse = func(conditionNode AstNode, bodyNodes []AstNode, elseNodes []AstNode, hasElse bool) {
		{
			updatedConditionNode := conditionNode
			conditionStart := len(compiler.Bytes)

			if compiler.TargetGame == "thug2" {
				write(0x47)
				write(0x00) // 2 temporary bytes for branch size
				write(0x00)
			} else {
				write(0x25) // use If1 instead of If2
			}

			writeBytecodeForNode(updatedConditionNode)
			for _, bodyNode := range bodyNodes {
				writeBytecodeForNode(bodyNode)
			}
			end := len(compiler.Bytes)
			size := end - conditionStart
			if hasElse {
				size += 2
			}
			if compiler.TargetGame == "thug2" {
				writeLittleUint16Index(uint16(size), conditionStart+1)
			}
		}
		if hasElse {
			start := len(compiler.Bytes)
			if compiler.TargetGame == "thug2" {
				write(0x48)
				write(0x00) // 2 temporary bytes for branch size
				write(0x00)
			} else {
				write(0x26) // use Else1 instead of Else2
			}
			for _, bodyNode := range elseNodes {
				writeBytecodeForNode(bodyNode)
			}
			end := len(compiler.Bytes)
			if compiler.TargetGame == "thug2" {
				writeLittleUint16Index(uint16(end-start), start+1)
			}
		}
		write(0x28)
	}

	writeNameTableEntry := func(checksum uint32, name string) {
		write(0x2B)
		writeLittleUint32(checksum)
		write([]byte(name)...)
		write(0)
	}

	// -----------

	writeBytecodeForNode(compiler.RootAstNode)

	if !compiler.RemoveChecksums {
		if len(nameTableOrder) > 0 {
			// Emit in the explicit order from __register_checksums__ (original
			// table order, which THUG2 requires). Append any names referenced in
			// the body but somehow not declared, so nothing is dropped.
			for _, name := range nameTableOrder {
				writeNameTableEntry(nameTable[name], name)
			}
			for name, checksum := range nameTable {
				if !nameTableOrderSeen[name] {
					writeNameTableEntry(checksum, name)
				}
			}
		} else {
			for name, checksum := range nameTable {
				writeNameTableEntry(checksum, name)
			}
		}
	}
	write(0)
}

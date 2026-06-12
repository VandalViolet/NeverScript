package decompiler

import (
    "encoding/binary"
    "errors"
    "fmt"
    "math"
    "os"
    "regexp"
    "strconv"
    "strings"
    "unicode"
)

var nsTrace = os.Getenv("NS_TRACE") != ""
var traceWriter = os.Stderr

const (
    Byte_EndOfFile         = 0x0
    Byte_NewLine           = 0x1
    Byte_NewLineWithNumber = 0x2
    Byte_Struct            = 0x3
    Byte_EndStruct         = 0x4
    Byte_Array             = 0x5
    Byte_EndArray          = 0x6
    Byte_Equals            = 0x7
    Byte_Dot               = 0x8
    Byte_Comma             = 0x9
    Byte_Minus             = 0xA
    Byte_Plus              = 0xB
    Byte_Divide            = 0xC
    Byte_Multiply          = 0xD
    Byte_Parenthesis       = 0xE
    Byte_EndParenthesis    = 0xF
    Byte_EqualTo           = 0x11
    Byte_LessThan          = 0x12
    Byte_LessThanEqual     = 0x13
    Byte_GreaterThan       = 0x14
    Byte_GreaterThanEqual  = 0x15
    Byte_Checksum          = 0x16
    Byte_Integer           = 0x17
    Byte_Float             = 0x1A
    Byte_String            = 0x1B
    Byte_LocalString       = 0x1C
    Byte_Vector            = 0x1E
    Byte_Pair              = 0x1F
    Byte_While             = 0x20
    Byte_EndWhile          = 0x21
    Byte_Break             = 0x22
    Byte_Script            = 0x23
    Byte_EndScript         = 0x24
    Byte_If                = 0x25
    Byte_Else              = 0x26
    Byte_EndIf             = 0x28
    Byte_Return            = 0x29
    Byte_ChecksumEntry     = 0x2B
    Byte_AllArguments      = 0x2C
    Byte_Local             = 0x2D
    Byte_LongJump          = 0x2E
    Byte_Random            = 0x2F
    Byte_RandomRange       = 0x30
    Byte_Or                = 0x32
    Byte_And               = 0x33
    Byte_Xor               = 0x34
    Byte_Not               = 0x39
    Byte_Switch            = 0x3C
    Byte_EndSwitch         = 0x3D
    Byte_Case              = 0x3E
    Byte_Default           = 0x3F
    Byte_RandomNoRepeat    = 0x40
    Byte_Colon             = 0x42
    Byte_ShortIf           = 0x47
    Byte_ShortElse         = 0x48
    Byte_ShortBreak        = 0x49
)

func Decompile(qb []byte) (string, error) {

    var DecompileExpression func(int, int, bool, bool) (string, int, error)
    var DecompileBodyOfCode func(int, int, bool) (string, int, error)
    var DecompileArgument func(int, int, bool) (string, int, error)
    var checksumTable map[uint32]string

    // The trailing name table in ORIGINAL ORDER. THUG2's loader requires the
    // symbol table in its exact (hash-bucket dump) order — a reordered table loads
    // but breaks name-dependent UI like the on-screen combo score. We can't cheaply
    // reproduce Neversoft's hash order for arbitrary symbol sets, so we capture the
    // original order here and re-emit it verbatim via __register_checksums__,
    // byte-faithful for round-trips and value-mods.
    var tableOrder []string

    // Upper bound (exclusive) for DecompileBodyOfCode. Defaults to the whole file;
    // temporarily tightened when decompiling a random's LAST branch (which has no
    // terminating longjump) so it stops at the random's true end instead of running
    // on into trailing post-random code at the enclosing scope.
    bodyEndLimit := len(qb)

    GetByte := func(index int) (byte, error) {
        if index >= len(qb) {
            //err := errors.New(fmt.Sprintf("Index 0x%x out of range", index))
            //log.Panic(err)
            return 0, nil
        }
        return qb[index], nil
    }

    GetBytes := func(index, size int) ([]byte, error) {
        if index+size > len(qb) {
            return []byte{}, errors.New(fmt.Sprintf("Index 0x%x out of range", index+size-1))
        }
        return qb[index : index+size], nil
    }

    GetChecksumTable := func() (map[uint32]string, error) {
        // THUG2 appends a trailing block of ChecksumEntry (0x2b) records —
        // <0x2b><hash u32><name nul-terminated> — after the script body. It maps
        // each symbol hash back to its source name so the body's bare 0x16 hash
        // references can be decompiled by name.
        //
        // The previous implementation scanned the file BACKWARDS looking for 0x2b
        // bytes. That is fundamentally ambiguous: 0x2b also occurs *inside* hash
        // data (e.g. crc 0xe2c4e22b is stored little-endian as 2b e2 c4 e2). The
        // backward scan would hit that embedded 0x2b first, parse a shifted bogus
        // entry, then set index = startOfChecksum-1 and step PAST the real opcode —
        // silently dropping the symbol (AU_SFX_Waves01 and every other crc whose
        // low byte is 0x2b). A symbol referenced in the body but missing from the
        // regenerated table on recompile is a prime load-freeze suspect.
        //
        // Instead, find the table boundary with the real opcode walker — a throwaway
        // body pass with an empty table; name resolution never changes how many
        // bytes a token consumes, so bytesRead is the exact table start — then
        // forward-parse the table, which is unambiguous (the hash is read as 4 raw
        // bytes and can never be mistaken for an opcode).
        checksumTable = make(map[uint32]string)
        _, tableStart, err := DecompileBodyOfCode(0, 0, true)
        if err != nil {
            return nil, err
        }

        table := make(map[uint32]string)
        index := tableStart

        for index < len(qb) {
            b, err := GetByte(index)
            if err != nil {
                return nil, err
            }
            if b != Byte_ChecksumEntry {
                break // trailing padding / end of table
            }
            index++

            checksumBytes, err := GetBytes(index, 4)
            if err != nil {
                return nil, err
            }
            checksum := binary.LittleEndian.Uint32(checksumBytes)
            index += 4
            checksumNameStartIndex := index

            // scan null-terminated name
            for {
                nextByte, err := GetByte(index)
                if err != nil {
                    return nil, err
                }
                if nextByte == 0 {
                    index++
                    break
                }
                index++
            }
            checksumName := string(qb[checksumNameStartIndex : index-1])

            // sanity check, may not be a printable checksum
            isPrintable := len(checksumName) > 0
            for _, c := range checksumName {
                if !unicode.IsNumber(c) && !unicode.IsLetter(c) && c != ' ' && c != '_' {
                    isPrintable = false
                    break
                }
            }
            if isPrintable {
                table[checksum] = checksumName
                tableOrder = append(tableOrder, checksumName)
            }
        }
        return table, nil
    }

    Indent := func(indentationLevel int, text string) string {
        return strings.Repeat("    ", indentationLevel) + text
    }

    TrimWhitespace := func(text string) string {
        isEntirelyWhitespace, _ := regexp.MatchString(`^ *$`, text)
        if isEntirelyWhitespace {
            return ""
        }
        return strings.Trim(text, " ")
    }

    FormatFloat := func(floatBytes []byte) string {
        floatAsInteger := binary.LittleEndian.Uint32(floatBytes)
        floatValue := math.Float32frombits(floatAsInteger)
        floatString := strconv.FormatFloat(float64(floatValue), 'f', -1, 32)
        if !strings.Contains(floatString, ".") {
            floatString += ".0"
        }
        return floatString
    }

    DecompilerError := func(message string, b byte, offset int) error {
        return errors.New(fmt.Sprintf("%s - 0x%x byte (offset 0x%x)", message, b, offset))
    }

    DecompileNewLineWithNumber := func(index int) (string, int, error) {
        //lineNumber := int(binary.LittleEndian.Uint32(qb[index : index+4]))
        //result := fmt.Sprintf("\n/* Line number 0x%x */", lineNumber)
        return "\n", 5, nil
    }

    DecompileConsecutiveNewLines := func(index int) (string, int, error) {
        initialIndex := index
        var newLinesAfterEqualsCode strings.Builder
        for {
            nextByte, err := GetByte(index)
            if err != nil {
                return "", 0, err
            }
            if nextByte == Byte_NewLine {
                index++
                newLinesAfterEqualsCode.WriteString("\n")
            } else if nextByte == Byte_NewLineWithNumber {
                newLineCode, bytesRead, err := DecompileNewLineWithNumber(index)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead
                newLinesAfterEqualsCode.WriteString(newLineCode)
            } else {
                break
            }
        }
        return newLinesAfterEqualsCode.String(), index - initialIndex, nil
    }

    DecompileString := func(index int) (string, int, error) {
        initialIndex := index
        index++
        lengthBytes := qb[index : index+4]
        length := int(binary.LittleEndian.Uint32(lengthBytes))
        index += 4
        stringBytes := qb[index : index+length]

        stringString := string(stringBytes)
        stringString = strings.ReplaceAll(stringString, "\\", "\\\\")
        stringString = strings.ReplaceAll(stringString, "\"", "\\\"")
        newLength := len(stringString)
        newStringBytes := []byte(stringString)

        index += length
        // warning: if string contains new-line bytes, it might produce code that doesn't compile
        return fmt.Sprintf(`"%s"`, string(newStringBytes[:newLength-1])), index - initialIndex, nil
    }

    DecompileChecksum := func(index int) (string, int, error) {
        initialIndex := index

        b, err := GetByte(index)
        if err != nil {
            return "", 0, err
        }

        isLocal := b == Byte_Local
        if isLocal {
            index++
        }

        // Validate that we are actually at a checksum opcode. Callers such as
        // DecompileAssignment (via DecompileArgument) speculatively try to parse
        // a checksum and rely on a clean error to fall back; without this check a
        // non-checksum byte (e.g. a newline) is consumed as 4 bytes of garbage,
        // which can false-positive as an assignment and derail the whole parse.
        opByte, err := GetByte(index)
        if err != nil {
            return "", 0, err
        }
        if opByte != Byte_Checksum {
            return "", 0, DecompilerError("Expected checksum byte", opByte, index)
        }

        index++
        checksumBytes := qb[index : index+4]
        index += 4

        var checksumCode string
        checksum := binary.LittleEndian.Uint32(checksumBytes)
        if checksumName, found := checksumTable[checksum]; found {
            if strings.Contains(checksumName, " ") {
                checksumCode = "`" + checksumName + "`"
            } else {
                checksumCode = checksumName
            }
        } else {
            checksumCode = fmt.Sprintf("#%02x%02x%02x%02x", checksumBytes[0], checksumBytes[1], checksumBytes[2], checksumBytes[3])
        }

        if isLocal {
            checksumCode = "<" + checksumCode + ">"
        }

        return checksumCode, index - initialIndex, nil
    }

    DecompilePair := func(index, indentationLevel int) (string, int, error) {
        initialIndex := index
        index++

        xBytes, err := GetBytes(index, 4)
        if err != nil {
            return "", 0, err
        }
        index += 4

        yBytes, err := GetBytes(index, 4)
        if err != nil {
            return "", 0, err
        }
        index += 4

        return fmt.Sprintf("(%s, %s)", FormatFloat(xBytes), FormatFloat(yBytes)), index - initialIndex, nil
    }

    DecompileAssignment := func(index, indentationLevel int, shouldPadEquals bool) (string, int, error) {
        initialIndex := index

        checksumCode, bytesRead, err := DecompileChecksum(index)
        if err != nil {
            return "", 0, err
        }
        index += bytesRead

        nextByte, err := GetByte(index)
        if err != nil {
            return "", 0, err
        }

        if nextByte != Byte_Equals {
            return "", 0, DecompilerError("No '=' in assignment", nextByte, index)
        }
        index++

        newLinesAfterEqualsCode, bytesRead, err := DecompileConsecutiveNewLines(index)
        if err != nil {
            return "", 0, err
        }
        index += bytesRead

        secondExpressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
        if err != nil {
            return "", 0, err
        }
        index += bytesRead

        var format string
        if shouldPadEquals {
            format = "%s = %s%s"
        } else {
            format = "%s=%s%s"
        }

        assignmentCode := fmt.Sprintf(format, checksumCode, newLinesAfterEqualsCode, secondExpressionCode)
        return assignmentCode, index - initialIndex, nil
    }

    DecompileBodyOfCode = func(index, indentationLevel int, shouldPadEquals bool) (string, int, error) {
        var currentLineCode strings.Builder
        var bodyOfCode strings.Builder
        flushCurrentLine := func() {
            bodyOfCode.WriteString(Indent(indentationLevel, currentLineCode.String()))
            currentLineCode.Reset()
        }

        initialIndex := index

        firstIteration := true
        for {
            // Stop at the active body limit (set while decompiling a random's last
            // branch) — treated exactly like hitting a scope terminator.
            if index >= bodyEndLimit {
                break
            }

            b, err := GetByte(index)
            if err != nil {
                return "", 0, err
            }

            if nsTrace {
                fmt.Fprintf(traceWriter, "[trace] idx=0x%x byte=0x%x depth=%d\n", index, b, indentationLevel)
            }

            appendSpace := !firstIteration && b != Byte_Comma && currentLineCode.Len() > 0
            if appendSpace {
                currentLineCode.WriteString(" ")
            }

            if b == Byte_NewLine {
                currentLineCode.WriteString("\n")
                flushCurrentLine()
                index++
            } else if b == Byte_NewLineWithNumber {
                newLineCode, bytesRead, err := DecompileNewLineWithNumber(index)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead
                currentLineCode.WriteString(newLineCode)
                flushCurrentLine()
            } else if b == Byte_Comma {
                index++
                currentLineCode.WriteString(",")
            } else if b == Byte_Local || b == Byte_Checksum || b == Byte_Return {
                expressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
                //argumentCode, bytesRead, err := DecompileArgument(index, indentationLevel, shouldPadEquals)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead
                currentLineCode.WriteString(expressionCode)
            } else if b == Byte_Break {
                index++
                currentLineCode.WriteString("break")
            } else if b == Byte_ShortBreak {
                // THUG2 optimized break: opcode + 2-byte offset to break target.
                // If the offset (measured from this opcode) lands on an endswitch
                // byte, this is a switch case's terminating break: stop the case
                // body here and leave the opcode for the switch handler to consume.
                offsetBytes, err := GetBytes(index+1, 2)
                if err != nil {
                    return "", 0, err
                }
                target := index + int(binary.LittleEndian.Uint16(offsetBytes))
                if tb, _ := GetByte(target); tb == Byte_EndSwitch {
                    break
                }
                // Otherwise it's an ordinary break; the offset is recomputed on recompile.
                index++
                index += 2
                currentLineCode.WriteString("break")
            } else if b == Byte_ShortIf {
                // THUG2 optimized if: opcode + 2-byte offset to the matching
                // else/endif. We parse structurally (skipping the offset) and rely on
                // ShortElse/Else/EndIf as block terminators, exactly like the long form.
                index++
                index += 2

                conditionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                ifBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                nextByte, err := GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte == Byte_ShortElse || nextByte == Byte_Else {
                    index++
                    if nextByte == Byte_ShortElse {
                        index += 2 // skip short offset to endif
                    }

                    elseBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                    if err != nil {
                        return "", 0, err
                    }
                    index += bytesRead

                    currentLineCode.WriteString(fmt.Sprintf("if %s {%s", conditionCode, ifBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }

                    currentLineCode.WriteString(fmt.Sprintf("} else {%s", elseBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }

                    currentLineCode.WriteString("}")
                } else {
                    currentLineCode.WriteString(fmt.Sprintf("if %s {%s", conditionCode, ifBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }
                    currentLineCode.WriteString("}")
                }

                nextByte, err = GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte != Byte_EndIf {
                    return "", 0, DecompilerError("No endif byte (short if)", nextByte, index)
                }
                index++
            } else if b == Byte_If {
                index++

                conditionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                ifBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                nextByte, err := GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte == Byte_Else {
                    index++

                    elseBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                    if err != nil {
                        return "", 0, err
                    }

                    index += bytesRead

                    currentLineCode.WriteString(fmt.Sprintf("if %s {%s", conditionCode, ifBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }

                    currentLineCode.WriteString(fmt.Sprintf("} else {%s", elseBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }

                    currentLineCode.WriteString("}")
                } else {
                    currentLineCode.WriteString(fmt.Sprintf("if %s {%s", conditionCode, ifBodyCode))
                    if strings.Contains(currentLineCode.String(), "\n") {
                        flushCurrentLine()
                    }
                    currentLineCode.WriteString("}")
                }

                nextByte, err = GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte != Byte_EndIf {
                    return "", 0, DecompilerError("No endif byte", nextByte, index)
                }
                index++
            } else if b == Byte_While {
                index++

                whileBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, shouldPadEquals)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                currentLineCode.WriteString(fmt.Sprintf("while {%s", whileBodyCode))
                if strings.Contains(currentLineCode.String(), "\n") {
                    flushCurrentLine()
                }
                currentLineCode.WriteString("}")

                nextByte, err := GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte != Byte_EndWhile {
                    return "", 0, DecompilerError("No endwhile byte", nextByte, index)
                }
                index++
            } else if b == Byte_Script {
                index++

                scriptNameCode, bytesRead, err := DecompileChecksum(index)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                scriptBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead

                nextByte, err := GetByte(index)
                if err != nil {
                    return "", 0, err
                }

                if nextByte != Byte_EndScript {
                    return "", 0, DecompilerError("No endscript byte", nextByte, index)
                }
                index++

                currentLineCode.WriteString(fmt.Sprintf("script %s {%s", scriptNameCode, scriptBodyCode))
                if strings.Contains(currentLineCode.String(), "\n") {
                    flushCurrentLine()
                }
                currentLineCode.WriteString("}")
            } else if b == Byte_Switch {
                // THUG2 switch/case (0x3C value (0x3E 0x49<off> caseValue body)* (0x3F body)? 0x3D).
                // The NeverScript compiler has no switch support and the 0x49
                // case-break offsets are awkward to re-emit, so we lower the
                // switch to an equivalent if/elseif chain (semantically identical
                // for the switch-on-variable form these scripts use), which the
                // compiler handles via its proven if/else path.
                index++

                switchVariableCode, bytesRead, err := DecompileChecksum(index)
                if err != nil {
                    // THUG2 also allows switching on a parenthesised expression
                    // (e.g. `switch (<expr>)`), which is not a bare checksum.
                    // Fall back to a full expression parse; the bare-checksum
                    // case above keeps existing output byte-identical.
                    switchVariableCode, bytesRead, err = DecompileExpression(index, indentationLevel, false, false)
                    if err != nil {
                        return "", 0, err
                    }
                }
                index += bytesRead

                var caseValues []string
                var caseBodies []string
                hasDefault := false
                defaultBody := ""

                for {
                    _, bytesRead, err := DecompileConsecutiveNewLines(index)
                    if err != nil {
                        return "", 0, err
                    }
                    index += bytesRead

                    nb, err := GetByte(index)
                    if err != nil {
                        return "", 0, err
                    }

                    if nb == Byte_EndSwitch {
                        index++
                        break
                    } else if nb == Byte_Case {
                        index++
                        // skip the case-intro short-break (0x49 + 2-byte offset)
                        if sb, _ := GetByte(index); sb == Byte_ShortBreak {
                            index += 3
                        }
                        valueCode, br, err := DecompileExpression(index, indentationLevel, false, false)
                        if err != nil {
                            return "", 0, err
                        }
                        index += br

                        bodyCode, br, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                        if err != nil {
                            return "", 0, err
                        }
                        index += br

                        // consume a trailing end-of-case short-break if present
                        if sb, _ := GetByte(index); sb == Byte_ShortBreak {
                            index += 3
                        }

                        caseValues = append(caseValues, valueCode)
                        caseBodies = append(caseBodies, bodyCode)
                    } else if nb == Byte_Default {
                        index++
                        bodyCode, br, err := DecompileBodyOfCode(index, indentationLevel+1, true)
                        if err != nil {
                            return "", 0, err
                        }
                        index += br
                        if sb, _ := GetByte(index); sb == Byte_ShortBreak {
                            index += 3
                        }
                        hasDefault = true
                        defaultBody = bodyCode
                    } else {
                        return "", 0, DecompilerError("Unexpected byte in switch body", nb, index)
                    }
                }

                // build the nested if/elseif chain from innermost outward
                chain := ""
                hasTail := false
                tail := ""
                if hasDefault {
                    tail = fmt.Sprintf("{\n%s\n}", defaultBody)
                    hasTail = true
                }
                for i := len(caseValues) - 1; i >= 0; i-- {
                    cond := fmt.Sprintf("(%s = %s)", switchVariableCode, caseValues[i])
                    ifPart := fmt.Sprintf("if %s {\n%s\n}", cond, caseBodies[i])
                    if hasTail {
                        chain = fmt.Sprintf("%s else %s", ifPart, tail)
                    } else {
                        chain = ifPart
                    }
                    tail = fmt.Sprintf("{\n%s\n}", chain)
                    hasTail = true
                }

                currentLineCode.WriteString(chain)
                if strings.Contains(currentLineCode.String(), "\n") {
                    flushCurrentLine()
                }
            } else if b == Byte_Case || b == Byte_Default {
                // case/default only appear inside a switch body, which is parsed
                // explicitly above; here they terminate the current (case) body.
                break
            } else if expressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals); err == nil {
                currentLineCode.WriteString(expressionCode)
                index += bytesRead
            } else if b == Byte_EndScript || b == Byte_EndStruct || b == Byte_EndArray || b == Byte_EndIf || b == Byte_Else || b == Byte_ShortElse || b == Byte_EndWhile || b == Byte_EndSwitch || b == Byte_LongJump || b == Byte_EndOfFile || b == Byte_ChecksumEntry {
                break
            } else {
                return "", 0, DecompilerError("Byte not recognised in body of code", b, index)
            }

            firstIteration = false
        }

        flushCurrentLine()
        return TrimWhitespace(bodyOfCode.String()), index - initialIndex, nil
    }

    DecompileArgument = func(index, indentationLevel int, shouldPadEquals bool) (string, int, error) {
        initialIndex := index

        assignmentCode, bytesRead, err := DecompileAssignment(index, indentationLevel, shouldPadEquals)
        if err == nil {
            // argument is an assignment e.g. x=3
            index += bytesRead
            return assignmentCode, index - initialIndex, nil
        } else {
            // argument is just an expression e.g. x
            expressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead
            return expressionCode, index - initialIndex, nil
        }
    }

    DecompileAtom := func(index, indentationLevel int, allowInvocationArguments, shouldPadEquals bool) (string, int, error) {
        initialIndex := index

        b, err := GetByte(index)
        if err != nil {
            return "", 0, err
        }

        if b == Byte_Local || b == Byte_Checksum || b == Byte_Return {
            var checksumOrReturnCode string
            if b == Byte_Return {
                checksumOrReturnCode = "return"
                index++
            } else {
                checksumCode, bytesRead, err := DecompileChecksum(index)
                if err != nil {
                    return "", 0, err
                }
                index += bytesRead
                checksumOrReturnCode = checksumCode
            }

            var argumentCodeArray []string
            if allowInvocationArguments {
                for {
                    argumentCode, bytesRead, err := DecompileArgument(index, indentationLevel, false)
                    if err != nil {
                        break
                    }
                    argumentCodeArray = append(argumentCodeArray, argumentCode)
                    index += bytesRead
                }
            }

            if len(argumentCodeArray) == 0 {
                return checksumOrReturnCode, index - initialIndex, nil
            } else {
                argumentsCode := strings.Join(argumentCodeArray, " ")
                return fmt.Sprintf("%s %s", checksumOrReturnCode, argumentsCode), index - initialIndex, nil
            }
        } else if b == Byte_String {
            stringCode, bytesRead, err := DecompileString(index)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead
            return stringCode, index - initialIndex, nil
        } else if b == Byte_LocalString {
            stringCode, bytesRead, err := DecompileString(index)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead
            return stringCode, index - initialIndex, nil
        } else if b == Byte_Integer {
            index++
            integerBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            integer := binary.LittleEndian.Uint32(integerBytes)
            index += 4
            return fmt.Sprintf("%d", int32(integer)), index - initialIndex, nil
        } else if b == Byte_Not {
            index++
            expressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, true)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead
            return fmt.Sprintf("! %s", expressionCode), index - initialIndex, nil
        } else if b == Byte_Parenthesis {
            index++

            expressionCode, bytesRead, err := DecompileExpression(index, indentationLevel, true, true)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            nextByte, err := GetByte(index)
            if err != nil {
                return "", 0, err
            }

            if nextByte != Byte_EndParenthesis {
                return "", 0, DecompilerError("No endparenthesis byte", nextByte, index)
            }
            index++

            return fmt.Sprintf("(%s)", expressionCode), index - initialIndex, nil
        } else if b == Byte_Struct {
            index++

            structBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, false)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            nextByte, err := GetByte(index)
            if err != nil {
                return "", 0, err
            }

            if nextByte != Byte_EndStruct {
                return "", 0, DecompilerError("No endstruct byte", nextByte, index)
            }
            index++

            var structClosingBraceCode string
            if strings.HasSuffix(structBodyCode, "\n") {
                structClosingBraceCode = Indent(indentationLevel, "}")
            } else {
                structClosingBraceCode = "}"
            }

            return fmt.Sprintf("{%s%s", structBodyCode, structClosingBraceCode), index - initialIndex, nil
        } else if b == Byte_Array {
            index++

            arrayBodyCode, bytesRead, err := DecompileBodyOfCode(index, indentationLevel+1, false)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            nextByte, err := GetByte(index)
            if err != nil {
                return "", 0, err
            }

            if nextByte != Byte_EndArray {
                return "", 0, DecompilerError("No endarray byte", nextByte, index)
            }
            index++

            var arrayClosingBracketCode string
            if strings.HasSuffix(arrayBodyCode, "\n") {
                arrayClosingBracketCode = Indent(indentationLevel, "]")
            } else {
                arrayClosingBracketCode = "]"
            }

            return fmt.Sprintf("[%s%s", arrayBodyCode, arrayClosingBracketCode), index - initialIndex, nil
        } else if b == Byte_Float {
            index++

            floatBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            index += 4

            return FormatFloat(floatBytes), index - initialIndex, nil
        } else if b == Byte_Pair {
            pairCode, bytesRead, err := DecompilePair(index, indentationLevel)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return pairCode, index - initialIndex, nil
        } else if b == Byte_Vector {
            index++

            xBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            index += 4

            yBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            index += 4

            zBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            index += 4

            return fmt.Sprintf("(%s, %s, %s)", FormatFloat(xBytes), FormatFloat(yBytes), FormatFloat(zBytes)), index - initialIndex, nil
        } else if b == Byte_AllArguments {
            index++
            return "<...>", index - initialIndex, nil
        } else if b == Byte_Random || b == Byte_RandomNoRepeat {
            initialIndex := index
            randomKeyword := "random"
            if b == Byte_RandomNoRepeat {
                randomKeyword = "random2" // 0x40 variant (distinct opcode; preserve it)
            }
            index++

            numberOfBranchesBytes, err := GetBytes(index, 4)
            if err != nil {
                return "", 0, err
            }
            numberOfBranches := int(binary.LittleEndian.Uint32(numberOfBranchesBytes))
            index += 4

            // branch weights: one uint16 per branch (THUG2 format)
            branchWeights := make([]int, numberOfBranches)
            for i := 0; i < numberOfBranches; i++ {
                weightBytes, err := GetBytes(index, 2)
                if err != nil {
                    return "", 0, err
                }
                branchWeights[i] = int(binary.LittleEndian.Uint16(weightBytes))
                index += 2
            }

            branchOffsets := make([]int, numberOfBranches)
            for i := 0; i < numberOfBranches; i++ {
                branchSizeBytes, err := GetBytes(index, 4)
                if err != nil {
                    return "", 0, err
                }
                branchSize := int(binary.LittleEndian.Uint32(branchSizeBytes))
                branchOffsets[i] = branchSize
                index += 4
            }

            // The random's true end = the longjump target shared by every non-last
            // branch (they all jump past the construct). The LAST branch has no such
            // longjump, so without this bound DecompileBodyOfCode would swallow the
            // trailing post-random code (e.g. a loop's `wait`) into it — which on
            // recompile pushes the branch exit-jumps past that code and freezes the
            // level on load. Compute the bound from the first branch's longjump.
            randomEnd := -1
            if numberOfBranches >= 2 {
                firstBranchStart := index + branchOffsets[1] - (4 * numberOfBranches) + (4 * 2)
                firstLongJumpPos := firstBranchStart - 5
                ljOffsetBytes, err := GetBytes(firstLongJumpPos+1, 4)
                if err == nil {
                    randomEnd = firstLongJumpPos + 5 + int(int32(binary.LittleEndian.Uint32(ljOffsetBytes)))
                }
            }

            // Per-random branch-0 newline: a 0x01 sits between the offset table
            // (which `index` now points just past) and the first branch in most —
            // but not all — randoms. Preserve it via a newline right after `{` so
            // the recompile is byte-identical (the offset table's +1 base depends
            // on it). The branch bodies themselves start AFTER this newline, so the
            // branch decompilation below is unaffected.
            hasBranch0Newline := index < len(qb) && qb[index] == Byte_NewLine

            branches := make([]string, numberOfBranches)
            lastBranchSize := 0
            for i := 0; i < numberOfBranches; i++ {
                branchIndex := index + branchOffsets[i] - (4 * numberOfBranches) + (4 * (i + 1))

                savedLimit := bodyEndLimit
                if i == numberOfBranches-1 && randomEnd > branchIndex && randomEnd <= len(qb) {
                    bodyEndLimit = randomEnd
                }
                branchCode, bytesRead, err := DecompileBodyOfCode(branchIndex, indentationLevel, shouldPadEquals)
                bodyEndLimit = savedLimit
                if err != nil {
                    return "", 0, err
                }
                branches[i] = branchCode
                lastBranchSize = bytesRead
            }

            index += branchOffsets[numberOfBranches-1]
            index += lastBranchSize

            for i, branch := range branches {
                // emit "<weight> { body }" so it round-trips through the compiler
                branches[i] = fmt.Sprintf("%d { %s }", branchWeights[i], branch)
            }

            // A newline right after `{` (vs a space) tells the compiler this random
            // has the branch-0 newline; no newline => it doesn't.
            branchSep := " "
            if hasBranch0Newline {
                branchSep = "\n"
            }
            return fmt.Sprintf("%s {%s%s }", randomKeyword, branchSep, strings.Join(branches, " ")), index - initialIndex, nil
        } else if b == Byte_RandomRange {
            index++

            pairCode, bytesRead, err := DecompilePair(index, indentationLevel)
            if err != nil {
                return "", 0, err
            }

            index += bytesRead

            return fmt.Sprintf("randomrange%s", pairCode), index - initialIndex, nil
        } else if b == Byte_Case {
            index++

            // THUG2 encodes a case as: 0x3E, then a 0x49 short-break (opcode +
            // 2-byte offset to the next case/endswitch), then the case value.
            if next, _ := GetByte(index); next == Byte_ShortBreak {
                index++
                index += 2
            }

            // TODO(brandon): not so sure about allowing invocation arguments here, might need to change
            caseCode, bytesRead, err := DecompileExpression(index, indentationLevel+1, true, false)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("case %s:", caseCode), index - initialIndex, nil
        } else if b == Byte_Default {
            index++

            return "default:", index - initialIndex, nil
        }

        return "", 0, DecompilerError("Not an atom", b, index)
    }

    DecompileExpression = func(index, indentationLevel int, allowInvocationArguments, shouldPadEquals bool) (string, int, error) {
        initialIndex := index

        atomCode, bytesRead, err := DecompileAtom(index, indentationLevel, allowInvocationArguments, shouldPadEquals)
        if err != nil {
            return "", 0, err
        }
        index += bytesRead

        // Postfix array subscript(s): atom[idx]. Binds tighter than infix operators.
        // Apply in value context, OR when the atom was parenthesised — `(expr)[idx]`
        // is unambiguously a subscript (a parenthesised value can't take an argument),
        // even inside an argument-allowing context like the body of another `(...)`.
        for !allowInvocationArguments || strings.HasPrefix(atomCode, "(") {
            sb, e := GetByte(index)
            if e != nil || sb != Byte_Array {
                break
            }
            subIndex := index + 1
            subscriptCode, subRead, e2 := DecompileExpression(subIndex, indentationLevel, false, shouldPadEquals)
            if e2 != nil {
                return "", 0, e2
            }
            subIndex += subRead
            eb, e3 := GetByte(subIndex)
            if e3 != nil || eb != Byte_EndArray {
                // not a well-formed subscript; leave the `[` for normal handling
                break
            }
            subIndex++
            atomCode = fmt.Sprintf("%s[%s]", atomCode, subscriptCode)
            index = subIndex
        }

        nextByte, err := GetByte(index)
        if err != nil {
            return atomCode, index - initialIndex, nil
        }

        if nextByte == Byte_Plus {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s + %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Minus {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s - %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Multiply {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s * %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Divide {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s / %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_And {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s and %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Or {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s or %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Xor {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s ^ %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Equals || nextByte == Byte_EqualTo {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                // HACK? Some scripts have no value on right-hand side of '='. Not sure why.
                nextExpression = ""
                bytesRead = 0
            }
            index += bytesRead

            var format string
            if shouldPadEquals {
                format = "%s = %s"
            } else {
                format = "%s=%s"
            }

            return fmt.Sprintf(format, atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Dot {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals) // maybe false?
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s.%s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_Colon {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s:%s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_GreaterThan {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s > %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_GreaterThanEqual {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s >= %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_LessThan {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, false, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s < %s", atomCode, nextExpression), index - initialIndex, nil
        } else if nextByte == Byte_LessThanEqual {
            index++

            nextExpression, bytesRead, err := DecompileExpression(index, indentationLevel, true, shouldPadEquals)
            if err != nil {
                return "", 0, err
            }
            index += bytesRead

            return fmt.Sprintf("%s <= %s", atomCode, nextExpression), index - initialIndex, nil
        }

        return atomCode, index - initialIndex, err
    }

    checksumTable, err := GetChecksumTable()
    if err != nil {
        return "", err
    }

    var output strings.Builder
    index := 0

    rootCode, bytesRead, err := DecompileBodyOfCode(0, 0, true)
    if err != nil {
        return "", err
    }
    index += bytesRead

    output.WriteString(rootCode)

    for {
        if index >= len(qb) {
            break
        }

        b, err := GetByte(index)
        if err != nil {
            return "", err
        }

        if b == Byte_EndOfFile {
            index++
            break
        } else if b == Byte_ChecksumEntry {
            index++
            index += 4
            for {
                nextByte, err := GetByte(index)
                if err != nil {
                    return "", err
                }
                if nextByte == 0 {
                    index++
                    break
                }
                index++
            }
        } else {
            break
        }
    }

    if index < len(qb) {
        nextByte, _ := GetByte(index)
        message := fmt.Sprintf("Did not finish decompiling.\n%s\n0x%x/0x%x bytes decompiled.\nnext byte: 0x%x", output.String(), index, len(qb), nextByte)
        return "", errors.New(message)
    }

    // Re-emit the ENTIRE name table in original order via __register_checksums__.
    // This both preserves orphan names (declared but never referenced, e.g. printf)
    // AND pins the table to its exact original order, which THUG2's loader requires
    // (a reordered table blanks the on-screen combo score). The compiler emits the
    // trailing table from this list verbatim instead of from a Go-map (random order).
    var tableNames []string
    for _, name := range tableOrder {
        // The directive parser reads identifier/keyword tokens; names with spaces
        // (rare debug strings) can't round-trip — surface them instead of corrupting.
        if strings.ContainsAny(name, " \t`") {
            output.WriteString(fmt.Sprintf("\n// WARNING: name-table entry not re-declared (non-identifier name): %q\n", name))
            continue
        }
        tableNames = append(tableNames, name)
    }
    if len(tableNames) > 0 {
        // No surrounding newlines: the directive emits no body bytecode, and any
        // newline around it WOULD compile to a stray 0x01 before the trailing name
        // table. The body's own trailing newline (already in `output`) separates it.
        output.WriteString("__register_checksums__ " + strings.Join(tableNames, " "))
    }

    return output.String(), nil
}

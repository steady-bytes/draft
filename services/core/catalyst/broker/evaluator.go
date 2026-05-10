package broker

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
)

// ceKind represents one of the three CESQL primitive types.
type ceKind int

const (
	kindString ceKind = iota
	kindInt
	kindBool
)

// ceVal is a dynamically-typed CESQL value.
type ceVal struct {
	kind    ceKind
	strVal  string
	intVal  int32
	boolVal bool
}

func sv(s string) ceVal { return ceVal{kind: kindString, strVal: s} }
func iv(i int32) ceVal  { return ceVal{kind: kindInt, intVal: i} }
func bv(b bool) ceVal   { return ceVal{kind: kindBool, boolVal: b} }

// matchesExpression evaluates expr against event and returns true when the
// expression is satisfied. A nil expression matches every event.
func matchesExpression(expr *acv1.Expression, event *acv1.CloudEvent) (bool, error) {
	if expr == nil {
		return true, nil
	}
	val, err := eval(expr, event)
	if err != nil {
		return false, err
	}
	b, _ := toBool(val)
	return b, nil
}

func eval(expr *acv1.Expression, event *acv1.CloudEvent) (ceVal, error) {
	if expr == nil {
		return bv(false), nil
	}
	switch e := expr.GetExpr().(type) {
	case *acv1.Expression_Literal:
		return evalLiteral(e.Literal), nil
	case *acv1.Expression_Attribute:
		return evalAttribute(e.Attribute, event), nil
	case *acv1.Expression_Unary:
		return evalUnary(e.Unary, event)
	case *acv1.Expression_Binary:
		return evalBinary(e.Binary, event)
	case *acv1.Expression_Like:
		return evalLike(e.Like, event)
	case *acv1.Expression_Exists:
		return evalExists(e.Exists, event), nil
	case *acv1.Expression_In:
		return evalIn(e.In, event)
	case *acv1.Expression_FunctionCall:
		return evalFunction(e.FunctionCall, event)
	default:
		return bv(false), nil
	}
}

func evalLiteral(lit *acv1.Literal) ceVal {
	switch v := lit.GetValue().(type) {
	case *acv1.Literal_IntegerValue:
		return iv(v.IntegerValue)
	case *acv1.Literal_BoolValue:
		return bv(v.BoolValue)
	case *acv1.Literal_StringValue:
		return sv(v.StringValue)
	default:
		return sv("")
	}
}

func evalAttribute(ref *acv1.AttributeRef, event *acv1.CloudEvent) ceVal {
	switch ref.GetName() {
	case "id":
		return sv(event.GetId())
	case "source":
		return sv(event.GetSource())
	case "specversion":
		return sv(event.GetSpecVersion())
	case "type":
		return sv(event.GetType())
	default:
		attrs := event.GetAttributes()
		if attrs == nil {
			return sv("")
		}
		v, ok := attrs[ref.GetName()]
		if !ok {
			return sv("")
		}
		return ceAttrToVal(v)
	}
}

func ceAttrToVal(v *acv1.CloudEvent_CloudEventAttributeValue) ceVal {
	switch a := v.GetAttr().(type) {
	case *acv1.CloudEvent_CloudEventAttributeValue_CeBoolean:
		return bv(a.CeBoolean)
	case *acv1.CloudEvent_CloudEventAttributeValue_CeInteger:
		return iv(a.CeInteger)
	case *acv1.CloudEvent_CloudEventAttributeValue_CeString:
		return sv(a.CeString)
	case *acv1.CloudEvent_CloudEventAttributeValue_CeUri:
		return sv(a.CeUri)
	case *acv1.CloudEvent_CloudEventAttributeValue_CeUriRef:
		return sv(a.CeUriRef)
	case *acv1.CloudEvent_CloudEventAttributeValue_CeBytes:
		return sv(base64.StdEncoding.EncodeToString(a.CeBytes))
	case *acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp:
		return sv(a.CeTimestamp.AsTime().Format(time.RFC3339Nano))
	default:
		return sv("")
	}
}

func evalUnary(u *acv1.UnaryExpr, event *acv1.CloudEvent) (ceVal, error) {
	operand, err := eval(u.GetOperand(), event)
	if err != nil {
		return bv(false), err
	}
	switch u.GetOp() {
	case acv1.UnaryOp_UNARY_OP_NOT:
		b, _ := toBool(operand)
		return bv(!b), nil
	case acv1.UnaryOp_UNARY_OP_NEGATE:
		n, _ := toInt(operand)
		return iv(-n), nil
	default:
		return bv(false), nil
	}
}

func evalBinary(b *acv1.BinaryExpr, event *acv1.CloudEvent) (ceVal, error) {
	left, err := eval(b.GetLeft(), event)
	if err != nil {
		return bv(false), err
	}

	// Short-circuit AND and OR before evaluating the right side.
	switch b.GetOp() {
	case acv1.BinaryOp_BINARY_OP_AND:
		lb, _ := toBool(left)
		if !lb {
			return bv(false), nil
		}
		right, err := eval(b.GetRight(), event)
		if err != nil {
			return bv(false), err
		}
		rb, _ := toBool(right)
		return bv(rb), nil

	case acv1.BinaryOp_BINARY_OP_OR:
		lb, _ := toBool(left)
		if lb {
			return bv(true), nil
		}
		right, err := eval(b.GetRight(), event)
		if err != nil {
			return bv(false), err
		}
		rb, _ := toBool(right)
		return bv(rb), nil
	}

	right, err := eval(b.GetRight(), event)
	if err != nil {
		return bv(false), err
	}

	switch b.GetOp() {
	case acv1.BinaryOp_BINARY_OP_XOR:
		lb, _ := toBool(left)
		rb, _ := toBool(right)
		return bv(lb != rb), nil

	case acv1.BinaryOp_BINARY_OP_EQ:
		return bv(cevalsEqual(left, right)), nil
	case acv1.BinaryOp_BINARY_OP_NEQ:
		return bv(!cevalsEqual(left, right)), nil

	case acv1.BinaryOp_BINARY_OP_LT:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return bv(li < ri), nil
	case acv1.BinaryOp_BINARY_OP_LTE:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return bv(li <= ri), nil
	case acv1.BinaryOp_BINARY_OP_GT:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return bv(li > ri), nil
	case acv1.BinaryOp_BINARY_OP_GTE:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return bv(li >= ri), nil

	case acv1.BinaryOp_BINARY_OP_ADD:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return iv(li + ri), nil
	case acv1.BinaryOp_BINARY_OP_SUB:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return iv(li - ri), nil
	case acv1.BinaryOp_BINARY_OP_MUL:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		return iv(li * ri), nil
	case acv1.BinaryOp_BINARY_OP_DIV:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		if ri == 0 {
			return iv(0), fmt.Errorf("division by zero")
		}
		return iv(li / ri), nil
	case acv1.BinaryOp_BINARY_OP_MOD:
		li, _ := toInt(left)
		ri, _ := toInt(right)
		if ri == 0 {
			return iv(0), fmt.Errorf("modulo by zero")
		}
		return iv(li % ri), nil

	default:
		return bv(false), nil
	}
}

func evalLike(l *acv1.LikeExpr, event *acv1.CloudEvent) (ceVal, error) {
	operand, err := eval(l.GetOperand(), event)
	if err != nil {
		return bv(false), err
	}
	s, _ := toString(operand)
	matched, err := likeMatch(l.GetPattern(), s)
	if err != nil {
		return bv(false), err
	}
	if l.GetNegated() {
		return bv(!matched), nil
	}
	return bv(matched), nil
}

// likeMatch converts a CESQL LIKE pattern to a regexp and tests s against it.
// % matches any sequence of characters, _ matches exactly one character,
// and \ escapes the next character literally.
func likeMatch(pattern, s string) (bool, error) {
	var re strings.Builder
	re.WriteString("(?s)^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '\\':
			if i+1 < len(pattern) {
				i++
				re.WriteString(regexp.QuoteMeta(string(pattern[i])))
			} else {
				re.WriteString(regexp.QuoteMeta("\\"))
			}
		case '%':
			re.WriteString(".*")
		case '_':
			re.WriteString(".")
		default:
			re.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	re.WriteString("$")
	return regexp.MatchString(re.String(), s)
}

func evalExists(e *acv1.ExistsExpr, event *acv1.CloudEvent) ceVal {
	switch e.GetAttribute() {
	case "id", "source", "specversion", "type":
		return bv(true)
	default:
		_, ok := event.GetAttributes()[e.GetAttribute()]
		return bv(ok)
	}
}

func evalIn(in *acv1.InExpr, event *acv1.CloudEvent) (ceVal, error) {
	operand, err := eval(in.GetOperand(), event)
	if err != nil {
		return bv(false), err
	}
	for _, valueExpr := range in.GetValues() {
		v, err := eval(valueExpr, event)
		if err != nil {
			continue
		}
		if cevalsEqual(operand, v) {
			return bv(!in.GetNegated()), nil
		}
	}
	return bv(in.GetNegated()), nil
}

func evalFunction(fc *acv1.FunctionCall, event *acv1.CloudEvent) (ceVal, error) {
	args := make([]ceVal, len(fc.GetArgs()))
	for i, arg := range fc.GetArgs() {
		v, err := eval(arg, event)
		if err != nil {
			return bv(false), err
		}
		args[i] = v
	}

	switch fc.GetFunction() {
	case acv1.BuiltinFunction_FUNCTION_LENGTH:
		if len(args) != 1 {
			return iv(0), fmt.Errorf("LENGTH requires 1 argument, got %d", len(args))
		}
		s, _ := toString(args[0])
		return iv(int32(len([]rune(s)))), nil

	case acv1.BuiltinFunction_FUNCTION_CONCAT:
		var sb strings.Builder
		for _, a := range args {
			s, _ := toString(a)
			sb.WriteString(s)
		}
		return sv(sb.String()), nil

	case acv1.BuiltinFunction_FUNCTION_CONCAT_WS:
		if len(args) < 1 {
			return sv(""), fmt.Errorf("CONCAT_WS requires at least 1 argument")
		}
		sep, _ := toString(args[0])
		parts := make([]string, 0, len(args)-1)
		for _, a := range args[1:] {
			s, _ := toString(a)
			parts = append(parts, s)
		}
		return sv(strings.Join(parts, sep)), nil

	case acv1.BuiltinFunction_FUNCTION_LOWER:
		if len(args) != 1 {
			return sv(""), fmt.Errorf("LOWER requires 1 argument")
		}
		s, _ := toString(args[0])
		return sv(strings.ToLower(s)), nil

	case acv1.BuiltinFunction_FUNCTION_UPPER:
		if len(args) != 1 {
			return sv(""), fmt.Errorf("UPPER requires 1 argument")
		}
		s, _ := toString(args[0])
		return sv(strings.ToUpper(s)), nil

	case acv1.BuiltinFunction_FUNCTION_TRIM:
		if len(args) != 1 {
			return sv(""), fmt.Errorf("TRIM requires 1 argument")
		}
		s, _ := toString(args[0])
		return sv(strings.TrimSpace(s)), nil

	case acv1.BuiltinFunction_FUNCTION_LEFT:
		if len(args) != 2 {
			return sv(""), fmt.Errorf("LEFT requires 2 arguments")
		}
		s, _ := toString(args[0])
		n, _ := toInt(args[1])
		runes := []rune(s)
		if n < 0 {
			return sv(s), fmt.Errorf("LEFT: n must be >= 0")
		}
		if int(n) >= len(runes) {
			return sv(s), nil
		}
		return sv(string(runes[:n])), nil

	case acv1.BuiltinFunction_FUNCTION_RIGHT:
		if len(args) != 2 {
			return sv(""), fmt.Errorf("RIGHT requires 2 arguments")
		}
		s, _ := toString(args[0])
		n, _ := toInt(args[1])
		runes := []rune(s)
		if n < 0 {
			return sv(s), fmt.Errorf("RIGHT: n must be >= 0")
		}
		if int(n) >= len(runes) {
			return sv(s), nil
		}
		return sv(string(runes[len(runes)-int(n):])), nil

	case acv1.BuiltinFunction_FUNCTION_SUBSTRING:
		if len(args) < 2 || len(args) > 3 {
			return sv(""), fmt.Errorf("SUBSTRING requires 2 or 3 arguments, got %d", len(args))
		}
		s, _ := toString(args[0])
		pos, _ := toInt(args[1])
		runes := []rune(s)
		l := int32(len(runes))
		var start int32
		switch {
		case pos > 0:
			start = pos - 1
		case pos < 0:
			start = l + pos
		default:
			return sv(""), nil
		}
		if start < 0 || start >= l {
			return sv(""), fmt.Errorf("SUBSTRING: position out of range")
		}
		if len(args) == 2 {
			return sv(string(runes[start:])), nil
		}
		length, _ := toInt(args[2])
		if length < 0 {
			return sv(""), fmt.Errorf("SUBSTRING: length must be >= 0")
		}
		end := start + length
		if end > l {
			end = l
		}
		return sv(string(runes[start:end])), nil

	case acv1.BuiltinFunction_FUNCTION_ABS:
		if len(args) != 1 {
			return iv(0), fmt.Errorf("ABS requires 1 argument")
		}
		n, _ := toInt(args[0])
		if n == -2147483648 {
			return iv(2147483647), fmt.Errorf("ABS: integer overflow for min int32")
		}
		if n < 0 {
			return iv(-n), nil
		}
		return iv(n), nil

	default:
		return bv(false), fmt.Errorf("unknown function: %v", fc.GetFunction())
	}
}

// ─── Type coercion ─────────────────────────────────────────────────────────────

func toBool(v ceVal) (bool, error) {
	switch v.kind {
	case kindBool:
		return v.boolVal, nil
	case kindInt:
		return v.intVal != 0, nil
	case kindString:
		switch strings.ToLower(strings.TrimSpace(v.strVal)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("cast error: %q is not a boolean", v.strVal)
		}
	}
	return false, nil
}

func toInt(v ceVal) (int32, error) {
	switch v.kind {
	case kindInt:
		return v.intVal, nil
	case kindBool:
		if v.boolVal {
			return 1, nil
		}
		return 0, nil
	case kindString:
		n, err := strconv.ParseInt(strings.TrimSpace(v.strVal), 10, 32)
		if err != nil {
			return 0, fmt.Errorf("cast error: %q is not an integer", v.strVal)
		}
		return int32(n), nil
	}
	return 0, nil
}

func toString(v ceVal) (string, error) {
	switch v.kind {
	case kindString:
		return v.strVal, nil
	case kindInt:
		return strconv.FormatInt(int64(v.intVal), 10), nil
	case kindBool:
		if v.boolVal {
			return "true", nil
		}
		return "false", nil
	}
	return "", nil
}

// cevalsEqual compares two ceVal values. Same-kind values compare directly;
// cross-kind values are coerced to string per the CESQL spec.
func cevalsEqual(a, b ceVal) bool {
	if a.kind == b.kind {
		switch a.kind {
		case kindString:
			return a.strVal == b.strVal
		case kindInt:
			return a.intVal == b.intVal
		case kindBool:
			return a.boolVal == b.boolVal
		}
	}
	as, _ := toString(a)
	bs, _ := toString(b)
	return as == bs
}

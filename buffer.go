package ajson

import (
	"io"
	"math"
	"strings"
)

type buffer struct {
	data   []byte
	length int
	index  int
}

const (
	quotes       byte = '"'
	quote        byte = '\''
	coma         byte = ','
	colon        byte = ':'
	backslash    byte = '\\'
	skipS        byte = ' '
	skipN        byte = '\n'
	skipR        byte = '\r'
	skipT        byte = '\t'
	bracketL     byte = '['
	bracketR     byte = ']'
	bracesL      byte = '{'
	bracesR      byte = '}'
	parenthesesL byte = '('
	parenthesesR byte = ')'
	dollar       byte = '$'
	at           byte = '@'
	dot          byte = '.'
	asterisk     byte = '*'
	plus         byte = '+'
	minus        byte = '-'
	division     byte = '/'
	exclamation  byte = '!'
	caret        byte = '^'
	signL        byte = '<'
	signG        byte = '>'
	signE        byte = '='
	ampersand    byte = '&'
	pipe         byte = '|'
	question     byte = '?'
)

type function func(*Node) (*Node, error)
type operation func(left *Node, right *Node) (*Node, error)
type rpn []string

var (
	_null  = []byte("null")
	_true  = []byte("true")
	_false = []byte("false")

	// Operator precedence
	// From https://golang.org/ref/spec#Operator_precedence
	//
	//	Precedence    Operator
	//	    5             *  /  %  <<  >>  &  &^
	//	    4             +  -  |  ^
	//	    3             ==  !=  <  <=  >  >=
	//	    2             &&
	//	    1             ||
	//
	// Arithmetic operators
	// From https://golang.org/ref/spec#Arithmetic_operators
	//
	//	+    sum                    integers, floats, complex values, strings
	//	-    difference             integers, floats, complex values
	//	*    product                integers, floats, complex values
	//	/    quotient               integers, floats, complex values
	//	%    remainder              integers
	//
	//	&    bitwise AND            integers
	//	|    bitwise OR             integers
	//	^    bitwise XOR            integers
	//	&^   bit clear (AND NOT)    integers
	//
	//	<<   left shift             integer << unsigned integer
	//	>>   right shift            integer >> unsigned integer
	//
	priority = map[string]int8{
		//fixme: "!":  7, // additional: factorial
		"**": 6, // additional: power
		"*":  5,
		"/":  5,
		"%":  5,
		"<<": 5,
		">>": 5,
		"&":  5,
		"&^": 5,
		"+":  4,
		"-":  4,
		"|":  4,
		"^":  4,
		"==": 3,
		"!=": 3,
		"<":  3,
		"<=": 3,
		">":  3,
		">=": 3,
		"&&": 2,
		"||": 1,
	}

	rightOp = map[string]bool{
		"**": true,
	}

	operations = map[string]operation{
		"**": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _floats(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "power", Numeric, math.Pow(lnum, rnum)), nil
		},
		"*": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _floats(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "multiply", Numeric, float64(lnum*rnum)), nil
		},
		"/": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _floats(left, right)
			if err != nil {
				return
			}
			if rnum == 0 {
				return nil, errorRequest("division by zero")
			}
			return varNode(nil, "division", Numeric, float64(lnum/rnum)), nil
		},
		"%": func(left *Node, right *Node) (result *Node, err error) {
			lnum, err := left.getInteger()
			if err != nil {
				return
			}
			rnum, err := left.getInteger()
			if err != nil {
				return
			}
			return varNode(nil, "remainder", Numeric, float64(lnum%rnum)), nil
		},
		"<<": func(left *Node, right *Node) (result *Node, err error) {
			lnum, err := left.getInteger()
			if err != nil {
				return
			}
			rnum, err := left.getUInteger()
			if err != nil {
				return
			}
			return varNode(nil, "left shift", Numeric, float64(lnum<<rnum)), nil
		},
		">>": func(left *Node, right *Node) (result *Node, err error) {
			lnum, err := left.getInteger()
			if err != nil {
				return
			}
			rnum, err := left.getUInteger()
			if err != nil {
				return
			}
			return varNode(nil, "right shift", Numeric, float64(lnum>>rnum)), nil
		},
		"&": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _ints(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "bitwise AND", Numeric, float64(lnum&rnum)), nil
		},
		"&^": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _ints(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "bit clear (AND NOT)", Numeric, float64(lnum&rnum)), nil
		},
		"+": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _floats(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "sum", Numeric, float64(lnum+rnum)), nil
		},
		"-": func(left *Node, right *Node) (result *Node, err error) {
			lnum, rnum, err := _floats(left, right)
			if err != nil {
				return
			}
			return varNode(nil, "sub", Numeric, float64(lnum-rnum)), nil
		},
		"|": func(left *Node, right *Node) (result *Node, err error) {
			if left.IsNumeric() && right.IsNumeric() {
				lnum, rnum, err := _ints(left, right)
				if err != nil {
					return nil, err
				}
				return varNode(nil, "bitwise OR", Numeric, float64(lnum|rnum)), nil
			}
			return nil, errorRequest("function 'bitwise OR' was called from non numeric node")
		},
		"^": func(left *Node, right *Node) (result *Node, err error) {
			if left.IsNumeric() && right.IsNumeric() {
				lnum, rnum, err := _ints(left, right)
				if err != nil {
					return nil, err
				}
				return varNode(nil, "bitwise XOR", Numeric, float64(lnum^rnum)), nil
			}
			return nil, errorRequest("function 'bitwise XOR' was called from non numeric node")
		},
		"==": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Eq(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "eq", Bool, bool(res)), nil
		},
		"!=": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Eq(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "neq", Bool, bool(!res)), nil
		},
		"<": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Le(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "le", Bool, bool(!res)), nil
		},
		"<=": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Leq(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "leq", Bool, bool(!res)), nil
		},
		">": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Ge(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "ge", Bool, bool(!res)), nil
		},
		">=": func(left *Node, right *Node) (result *Node, err error) {
			res, err := left.Geq(right)
			if err != nil {
				return nil, err
			}
			return varNode(nil, "geq", Bool, bool(!res)), nil
		},
		"&&": func(left *Node, right *Node) (result *Node, err error) {
			res := false
			lval, err := boolean(left)
			if err != nil {
				return nil, err
			}
			if lval {
				rval, err := boolean(right)
				if err != nil {
					return nil, err
				}
				res = rval
			}
			return varNode(nil, "AND", Bool, bool(!res)), nil
		},
		"||": func(left *Node, right *Node) (result *Node, err error) {
			res := true
			lval, err := boolean(left)
			if err != nil {
				return nil, err
			}
			if !lval {
				rval, err := boolean(right)
				if err != nil {
					return nil, err
				}
				res = rval
			}
			return varNode(nil, "OR", Bool, bool(!res)), nil
		},
	}

	functions = map[string]function{
		"sin": func(node *Node) (result *Node, err error) {
			if node.IsNumeric() {
				num, err := node.GetNumeric()
				if err != nil {
					return nil, err
				}
				return varNode(nil, "sin", Numeric, math.Sin(num)), nil
			}
			return nil, errorRequest("function 'sin' was called from non numeric node")
		},
		"cos": func(node *Node) (result *Node, err error) {
			if node.IsNumeric() {
				num, err := node.GetNumeric()
				if err != nil {
					return nil, err
				}
				return varNode(nil, "cos", Numeric, math.Cos(num)), nil
			}
			return nil, errorRequest("function 'cos' was called from non numeric node")
		},
		"length": func(node *Node) (result *Node, err error) {
			if node.IsArray() {
				return varNode(node, "length", Numeric, float64(node.Size())), nil
			}
			return nil, errorRequest("function 'length' was called from non array node")
		},
		"factorial": func(node *Node) (result *Node, err error) {
			num, err := node.getUInteger()
			if err != nil {
				return
			}
			return varNode(nil, "factorial", Numeric, float64(mathFactorial(num))), nil
		},
	}
	constants = map[string]*Node{
		"pi":    varNode(nil, "pi", Numeric, float64(math.Pi)),
		"e":     varNode(nil, "e", Numeric, float64(math.E)),
		"true":  varNode(nil, "true", Bool, true),
		"false": varNode(nil, "false", Bool, false),
		"null":  varNode(nil, "null", Null, nil),
	}
)

func newBuffer(body []byte) (b *buffer) {
	b = &buffer{
		length: len(body),
		data:   body,
	}
	return
}

func (b *buffer) current() (c byte, err error) {
	if b.index < b.length {
		return b.data[b.index], nil
	}
	return 0, io.EOF
}

func (b *buffer) next() (c byte, err error) {
	err = b.step()
	if err != nil {
		return 0, err
	}
	return b.data[b.index], nil
}

func (b *buffer) first() (c byte, err error) {
	for ; b.index < b.length; b.index++ {
		c = b.data[b.index]
		if !(c == skipS || c == skipR || c == skipN || c == skipT) {
			return c, nil
		}
	}
	return 0, io.EOF
}

func (b *buffer) backslash() (result bool) {
	for i := b.index - 1; i >= 0; i-- {
		if b.data[i] == backslash {
			result = !result
		} else {
			break
		}
	}
	return
}

func (b *buffer) skip(s byte) error {
	for ; b.index < b.length; b.index++ {
		if b.data[b.index] == s && !b.backslash() {
			return nil
		}
	}
	return io.EOF
}

func (b *buffer) skipAny(s map[byte]bool) error {
	for ; b.index < b.length; b.index++ {
		if s[b.data[b.index]] && !b.backslash() {
			return nil
		}
	}
	return io.EOF
}

func (b *buffer) numeric() error {
	var c byte
	find := 0
	for ; b.index < b.length; b.index++ {
		c = b.data[b.index]
		switch true {
		case c >= '0' && c <= '9':
			find |= 4
		case c == '.':
			if find&2 == 0 && find&8 == 0 { // exp part of numeric MUST contains only digits
				find &= 2
			} else {
				return errorSymbol(b)
			}
		case c == '+' || c == '-':
			if find == 0 || find == 8 {
				find |= 1
			} else {
				return errorSymbol(b)
			}
		case c == 'e' || c == 'E':
			if find&8 == 0 && find&4 != 0 { // exp without base part
				find = 8
			} else {
				return errorSymbol(b)
			}
		default:
			if find&4 != 0 {
				return nil
			}
			return errorSymbol(b)
		}
	}
	if find&4 != 0 {
		return io.EOF
	}
	return errorEOF(b)
}

func (b *buffer) string(search byte) error {
	err := b.step()
	if err != nil {
		return errorEOF(b)
	}
	if b.skip(search) != nil {
		return errorEOF(b)
	}
	return nil
}

func (b *buffer) null() error {
	return b.word(_null)
}

func (b *buffer) true() error {
	return b.word(_true)
}

func (b *buffer) false() error {
	return b.word(_false)
}

func (b *buffer) word(word []byte) error {
	var c byte
	max := len(word)
	index := 0
	for ; b.index < b.length; b.index++ {
		c = b.data[b.index]
		if c != word[index] && c != (word[index]-32) {
			return errorSymbol(b)
		}
		index++
		if index >= max {
			break
		}
	}
	if index != max {
		return errorEOF(b)
	}
	return nil
}

func (b *buffer) step() error {
	if b.index+1 < b.length {
		b.index++
		return nil
	}
	return io.EOF
}

// reads until the end of the token e.g.: `@.length`, `@['foo'].bar[(@.length - 1)].baz`
func (b *buffer) token() (err error) {
	var (
		c     byte
		stack = make([]byte, 0)
		start int
		find  bool
	)
tokenLoop:
	for ; b.index < b.length; b.index++ {
		c = b.data[b.index]
		switch {
		case c == quote:
			find = true
			start = b.index
			err = b.step()
			if err != nil {
				return b.errorEOF()
			}
			err = b.skip(quote)
			if err == nil || err == io.EOF {
				continue
			}
			b.index = start
		case c == bracketL:
			find = true
			stack = append(stack, c)
		case c == bracketR:
			find = true
			if len(stack) == 0 || stack[len(stack)-1] != bracketL {
				return b.errorSymbol()
			}
			stack = stack[:len(stack)-1]
		case c == parenthesesL:
			find = true
			stack = append(stack, c)
		case c == parenthesesR:
			find = true
			if len(stack) == 0 || stack[len(stack)-1] != parenthesesL {
				return b.errorSymbol()
			}
			stack = stack[:len(stack)-1]
		case c == dot || c == at || c == dollar || c == question || c == asterisk || (c >= 'A' && c <= 'z') || (c >= '0' && c <= '9'): // standard token name
			find = true
			continue
		case len(stack) != 0:
			find = true
			continue
		case c == minus || c == plus:
			if !find {
				find = true
				start = b.index
				err = b.numeric()
				if err == nil || err == io.EOF {
					b.index--
					continue
				}
				b.index = start
			}
			fallthrough
		default:
			break tokenLoop
		}
	}
	if len(stack) != 0 {
		return b.errorEOF()
	}
	if b.index >= b.length {
		return io.EOF
	}
	return nil
}

//  Builder for `Reverse Polish notation`
func (b *buffer) rpn() (result rpn, err error) {
	var (
		c        byte
		start    int
		temp     string
		current  string
		found    bool
		variable bool
		stack    = make([]string, 0)
	)
	for {
		c, err = b.first()
		if err != nil {
			break
		}
		switch true {
		case c == asterisk || c == division || c == minus || c == plus || c == caret || c == ampersand || c == pipe || c == signL || c == signG || c == signE || c == exclamation: // operations
			if variable {
				variable = false
				current = string(c)

				c, err = b.next()
				if err == nil {
					temp = current + string(c)
					if priority[temp] != 0 {
						current = temp
					} else {
						b.index--
					}
				} else {
					err = nil
				}

				for len(stack) > 0 {
					temp = stack[len(stack)-1]
					found = false
					if temp[0] >= 'A' && temp[0] <= 'z' { // function
						found = true
					} else if priority[temp] != 0 { // operation
						if priority[temp] > priority[current] {
							found = true
						} else if priority[temp] == priority[current] && !rightOp[temp] {
							found = true
						}
					}

					if found {
						stack = stack[:len(stack)-1]
						result = append(result, temp)
					} else {
						break
					}
				}
				stack = append(stack, current)
				break
			}
			if c != minus && c != plus {
				return nil, b.errorSymbol()
			}
			fallthrough // for numbers like `-1e6`
		case (c >= '0' && c <= '9') || c == '.': // numbers
			variable = true
			start = b.index
			err = b.numeric()
			if err != nil && err != io.EOF {
				return nil, err
			}
			current = string(b.data[start:b.index])
			result = append(result, current)
			if err != nil {
				err = nil
			} else {
				b.index--
			}
		case c == quote: // string
			variable = true
			start = b.index
			err = b.string(quote)
			if err != nil {
				return nil, b.errorEOF()
			}
			current = string(b.data[start : b.index+1])
			result = append(result, current)
		case c == dollar || c == at: // variable : like @.length , $.expensive, etc.
			variable = true
			start = b.index
			err = b.token()
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
			}
			current = string(b.data[start:b.index])
			result = append(result, current)
			if err != nil {
				err = nil
			} else {
				b.index--
			}
		case c == parenthesesL: // (
			variable = false
			current = string(c)
			stack = append(stack, current)
		case c == parenthesesR: // )
			variable = true
			found = false
			for len(stack) > 0 {
				temp = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if temp == "(" {
					found = true
					break
				}
				result = append(result, temp)
			}
			if !found { // have no parenthesesL
				return nil, errorRequest("formula has no left parentheses")
			}
		default: // prefix functions or etc.
			start = b.index
			variable = true
			for ; b.index < b.length; b.index++ {
				c = b.data[b.index]
				if c == parenthesesL { // function detection, example: sin(...), round(...), etc.
					variable = false
					break
				}
				if c < 'A' || c > 'z' {
					if !(c >= '0' && c <= '9') && c != '_' { // constants detection, example: true, false, null, PI, e, etc.
						break
					}
				}
			}
			current = strings.ToLower(string(b.data[start:b.index]))
			b.index--
			if !variable {
				if _, found = functions[current]; !found {
					return nil, errorRequest("wrong formula, '%s' is not a function", current)
				}
				stack = append(stack, current)
			} else {
				if _, found = constants[current]; !found {
					return nil, errorRequest("wrong formula, '%s' is not a constant", current)
				}
				result = append(result, current)
			}
		}
		err = b.step()
		if err != nil {
			break
		}
	}

	if err != io.EOF {
		return
	}
	err = nil

	for len(stack) > 0 {
		temp = stack[len(stack)-1]
		_, ok := functions[temp]
		if priority[temp] == 0 && !ok { // operations only
			return nil, errorRequest("wrong formula, '%s' is not an operation or function", temp)
		}
		result = append(result, temp)
		stack = stack[:len(stack)-1]
	}

	return
}

func (b *buffer) errorEOF() error {
	return errorEOF(b)
}

func (b *buffer) errorSymbol() error {
	return errorSymbol(b)
}

func mathFactorial(x uint) uint {
	if x == 0 {
		return 1
	}
	return x * mathFactorial(x-1)
}

func _floats(left, right *Node) (lnum, rnum float64, err error) {
	lnum, err = left.GetNumeric()
	if err != nil {
		return
	}
	rnum, err = right.GetNumeric()
	return
}

func _ints(left, right *Node) (lnum, rnum int, err error) {
	lnum, err = left.getInteger()
	if err != nil {
		return
	}
	rnum, err = right.getInteger()
	return
}

func _bools(left, right *Node) (lnum, rnum bool, err error) {
	lnum, err = left.GetBool()
	if err != nil {
		return
	}
	rnum, err = right.GetBool()
	return
}

func _nulls(left, right *Node) (lnum, rnum interface{}, err error) {
	lnum, err = left.GetNull()
	if err != nil {
		return
	}
	rnum, err = right.GetNull()
	return
}

func _strings(left, right *Node) (lnum, rnum string, err error) {
	lnum, err = left.GetString()
	if err != nil {
		return
	}
	rnum, err = right.GetString()
	return
}

func _arrays(left, right *Node) (lnum, rnum []*Node, err error) {
	lnum, err = left.GetArray()
	if err != nil {
		return
	}
	rnum, err = right.GetArray()
	return
}

func _objects(left, right *Node) (lnum, rnum map[string]*Node, err error) {
	lnum, err = left.GetObject()
	if err != nil {
		return
	}
	rnum, err = right.GetObject()
	return
}

func boolean(node *Node) (bool, error) {
	switch node.Type() {
	case Bool:
		return node.GetBool()
	case Numeric:
		res, err := node.GetNumeric()
		return res != 0, err
	case String:
		res, err := node.GetString()
		return res != "", err
	case Null:
		return false, nil
	case Array:
		fallthrough
	case Object:
		return !node.Empty(), nil
	}
	return false, nil
}

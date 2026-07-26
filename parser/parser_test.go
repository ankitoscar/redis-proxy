package parser

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestParserSuccess(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Value
	}{
		{
			name:  "Simple String OK",
			input: "+OK\r\n",
			expected: Value{
				Type: TypeSimpleString,
				Str:  "OK",
			},
		},
		{
			name:  "Simple String PONG",
			input: "+PONG\r\n",
			expected: Value{
				Type: TypeSimpleString,
				Str:  "PONG",
			},
		},
		{
			name:  "Error",
			input: "-ERR unknown command 'foobar'\r\n",
			expected: Value{
				Type: TypeError,
				Str:  "ERR unknown command 'foobar'",
			},
		},
		{
			name:  "Integer Positive",
			input: ":1000\r\n",
			expected: Value{
				Type: TypeInteger,
				Num:  1000,
			},
		},
		{
			name:  "Integer Zero",
			input: ":0\r\n",
			expected: Value{
				Type: TypeInteger,
				Num:  0,
			},
		},
		{
			name:  "Integer Negative",
			input: ":-123\r\n",
			expected: Value{
				Type: TypeInteger,
				Num:  -123,
			},
		},
		{
			name:  "Bulk String Standard",
			input: "$6\r\nfoobar\r\n",
			expected: Value{
				Type: TypeBulkString,
				Str:  "foobar",
			},
		},
		{
			name:  "Bulk String Empty",
			input: "$0\r\n\r\n",
			expected: Value{
				Type: TypeBulkString,
				Str:  "",
			},
		},
		{
			name:  "Bulk String Null",
			input: "$-1\r\n",
			expected: Value{
				Type: TypeBulkString,
				Null: true,
			},
		},
		{
			name:  "Bulk String with Newlines",
			input: "$12\r\nhello\r\nworld\r\n",
			expected: Value{
				Type: TypeBulkString,
				Str:  "hello\r\nworld",
			},
		},
		{
			name:  "Array Standard",
			input: "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n",
			expected: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "GET"},
					{Type: TypeBulkString, Str: "key"},
				},
			},
		},
		{
			name:  "Array Empty",
			input: "*0\r\n",
			expected: Value{
				Type:  TypeArray,
				Array: []Value{},
			},
		},
		{
			name:  "Array Null",
			input: "*-1\r\n",
			expected: Value{
				Type: TypeArray,
				Null: true,
			},
		},
		{
			name:  "Array Mixed Types",
			input: "*3\r\n:1\r\n+OK\r\n$-1\r\n",
			expected: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeInteger, Num: 1},
					{Type: TypeSimpleString, Str: "OK"},
					{Type: TypeBulkString, Null: true},
				},
			},
		},
		{
			name:  "Array Nested",
			input: "*2\r\n*1\r\n:5\r\n+nested\r\n",
			expected: Value{
				Type: TypeArray,
				Array: []Value{
					{
						Type: TypeArray,
						Array: []Value{
							{Type: TypeInteger, Num: 5},
						},
					},
					{Type: TypeSimpleString, Str: "nested"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(bytes.NewReader([]byte(tt.input)))
			val, err := p.Read()
			if err != nil {
				t.Fatalf("unexpected error parsing %q: %v", tt.input, err)
			}
			if !reflect.DeepEqual(val, tt.expected) {
				t.Errorf("parsing %q:\nexpected %+v\ngot      %+v", tt.input, tt.expected, val)
			}
		})
	}
}

func TestParserErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Empty Input",
			input: "",
		},
		{
			name:  "Invalid Type Prefix",
			input: "xOK\r\n",
		},
		{
			name:  "Simple String Missing CRLF",
			input: "+OK",
		},
		{
			name:  "Integer Missing CRLF",
			input: ":123",
		},
		{
			name:  "Integer Invalid Characters",
			input: ":12a3\r\n",
		},
		{
			name:  "Bulk String Invalid Length Format",
			input: "$abc\r\n",
		},
		{
			name:  "Bulk String Length Mismatch Short",
			input: "$5\r\nfoo\r\n",
		},
		{
			name:  "Bulk String Length Mismatch Long",
			input: "$3\r\nfoobar\r\n",
		},
		{
			name:  "Bulk String Missing Trailing CRLF",
			input: "$3\r\nfoo",
		},
		{
			name:  "Array Invalid Count Format",
			input: "*abc\r\n",
		},
		{
			name:  "Array Incomplete Elements",
			input: "*2\r\n$3\r\nGET\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(bytes.NewReader([]byte(tt.input)))
			_, err := p.Read()
			if err == nil {
				t.Errorf("expected error parsing malformed input %q, got nil", tt.input)
			}
		})
	}
}

func TestParserPipelining(t *testing.T) {
	input := "*1\r\n$4\r\nPING\r\n+PONG\r\n:42\r\n"
	p := NewParser(bytes.NewReader([]byte(input)))

	// 1. Read first value (Array with PING)
	val1, err := p.Read()
	if err != nil {
		t.Fatalf("failed to read first value: %v", err)
	}
	expected1 := Value{
		Type: TypeArray,
		Array: []Value{
			{Type: TypeBulkString, Str: "PING"},
		},
	}
	if !reflect.DeepEqual(val1, expected1) {
		t.Errorf("expected first value %+v, got %+v", expected1, val1)
	}

	// 2. Read second value (+PONG\r\n)
	val2, err := p.Read()
	if err != nil {
		t.Fatalf("failed to read second value: %v", err)
	}
	expected2 := Value{
		Type: TypeSimpleString,
		Str:  "PONG",
	}
	if !reflect.DeepEqual(val2, expected2) {
		t.Errorf("expected second value %+v, got %+v", expected2, val2)
	}

	// 3. Read third value (:42\r\n)
	val3, err := p.Read()
	if err != nil {
		t.Fatalf("failed to read third value: %v", err)
	}
	expected3 := Value{
		Type: TypeInteger,
		Num:  42,
	}
	if !reflect.DeepEqual(val3, expected3) {
		t.Errorf("expected third value %+v, got %+v", expected3, val3)
	}

	// 4. Read fourth value should return EOF error
	_, err = p.Read()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF error, got %v", err)
	}
}

func TestWriteValue(t *testing.T) {
	tests := []struct {
		name     string
		val      Value
		expected string
	}{
		{
			name: "Simple String",
			val: Value{
				Type: TypeSimpleString,
				Str:  "OK",
			},
			expected: "+OK\r\n",
		},
		{
			name: "Error",
			val: Value{
				Type: TypeError,
				Str:  "ERR error",
			},
			expected: "-ERR error\r\n",
		},
		{
			name: "Integer",
			val: Value{
				Type: TypeInteger,
				Num:  123,
			},
			expected: ":123\r\n",
		},
		{
			name: "Bulk String",
			val: Value{
				Type: TypeBulkString,
				Str:  "hello",
			},
			expected: "$5\r\nhello\r\n",
		},
		{
			name: "Bulk String Empty",
			val: Value{
				Type: TypeBulkString,
				Str:  "",
			},
			expected: "$0\r\n\r\n",
		},
		{
			name: "Bulk String Null",
			val: Value{
				Type: TypeBulkString,
				Null: true,
			},
			expected: "$-1\r\n",
		},
		{
			name: "Array Standard",
			val: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "GET"},
					{Type: TypeBulkString, Str: "key"},
				},
			},
			expected: "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n",
		},
		{
			name: "Array Null",
			val: Value{
				Type: TypeArray,
				Null: true,
			},
			expected: "*-1\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteValue(&buf, tt.val)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if buf.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, buf.String())
			}
		})
	}
}

func TestIsWriteCommand(t *testing.T) {
	tests := []struct {
		name     string
		val      Value
		expected bool
	}{
		{
			name: "SET is write command",
			val: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "SET"},
					{Type: TypeBulkString, Str: "key"},
					{Type: TypeBulkString, Str: "value"},
				},
			},
			expected: true,
		},
		{
			name: "set (lowercase) is write command",
			val: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "set"},
					{Type: TypeBulkString, Str: "key"},
					{Type: TypeBulkString, Str: "value"},
				},
			},
			expected: true,
		},
		{
			name: "GET is not write command",
			val: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "GET"},
					{Type: TypeBulkString, Str: "key"},
				},
			},
			expected: false,
		},
		{
			name: "PING is not write command",
			val: Value{
				Type: TypeArray,
				Array: []Value{
					{Type: TypeBulkString, Str: "PING"},
				},
			},
			expected: false,
		},
		{
			name:     "Non-array value is not write command",
			val:      Value{Type: TypeSimpleString, Str: "OK"},
			expected: false,
		},
		{
			name:     "Empty array is not write command",
			val:      Value{Type: TypeArray, Array: []Value{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := IsWriteCommand(tt.val)
			if res != tt.expected {
				t.Errorf("IsWriteCommand(%+v) = %v; expected %v", tt.val, res, tt.expected)
			}
		})
	}
}

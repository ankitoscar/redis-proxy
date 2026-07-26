package parser

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
)

type Type byte

const (
	TypeSimpleString Type = '+'
	TypeError        Type = '-'
	TypeInteger      Type = ':'
	TypeBulkString   Type = '$'
	TypeArray        Type = '*'
)

type Value struct {
	Type  Type
	Str   string
	Num   int
	Array []Value
	Null  bool
}

type Parser struct {
	reader *bufio.Reader
}

func NewParser(rd io.Reader) *Parser {
	return &Parser{reader: bufio.NewReader(rd)}
}

func (p *Parser) Read() (Value, error) {
	// Stub implementation for TDD.
	// This will compile but fail tests until implemented.
	firstByte, err := p.reader.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch Type(firstByte) {
	case TypeSimpleString:
		return p.readSimpleString()
	case TypeError:
		return p.readError()
	case TypeInteger:
		return p.readInteger()
	case TypeBulkString:
		return p.readBulkString()
	case TypeArray:
		return p.readArray()
	default:
		return Value{}, errors.New("unknown value type")
	}
}

func (p *Parser) readLine() ([]byte, error) {
	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' || line[len(line)-1] != '\n' {
		return nil, errors.New("invalid line ending")
	}
	return line[:len(line)-2], nil
}

func (p *Parser) readSimpleString() (Value, error) {
	line, err := p.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{
		Type: TypeSimpleString,
		Str:  string(line),
	}, nil
}

func (p *Parser) readError() (Value, error) {
	line, err := p.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{
		Type: TypeError,
		Str:  string(line),
	}, nil
}

func (p *Parser) readInteger() (Value, error) {
	line, err := p.readLine()
	if err != nil {
		return Value{}, err
	}
	num, err := strconv.Atoi(string(line))
	if err != nil {
		return Value{}, err
	}
	return Value{
		Type: TypeInteger,
		Num:  num,
	}, nil
}

func (p *Parser) readBulkString() (Value, error) {
	line, err := p.readLine()
	if err != nil {
		return Value{}, err
	}
	length, err := strconv.Atoi(string(line))
	if err != nil {
		return Value{}, err
	}

	if length == -1 {
		return Value{
			Type: TypeBulkString,
			Null: true,
		}, nil
	}

	if length < -1 {
		return Value{}, errors.New("invalid bulk string length")
	}

	// Read exactly length bytes, plus 2 bytes for the trailing \r\n
	buf := make([]byte, length+2)
	_, err = io.ReadFull(p.reader, buf)
	if err != nil {
		return Value{}, err
	}

	if buf[length] != '\r' || buf[length+1] != '\n' {
		return Value{}, errors.New("invalid bulk string trailing CRLF")
	}

	return Value{
		Type: TypeBulkString,
		Str:  string(buf[:length]),
	}, nil
}

func (p *Parser) readArray() (Value, error) {
	line, err := p.readLine()
	if err != nil {
		return Value{}, err
	}
	length, err := strconv.Atoi(string(line))
	if err != nil {
		return Value{}, err
	}

	if length == -1 {
		return Value{
			Type: TypeArray,
			Null: true,
		}, nil
	}

	if length < -1 {
		return Value{}, errors.New("invalid array length")
	}

	arr := make([]Value, 0, length)
	for i := 0; i < length; i++ {
		val, err := p.Read()
		if err != nil {
			return Value{}, err
		}
		arr = append(arr, val)
	}
	return Value{
		Type:  TypeArray,
		Array: arr,
	}, nil
}

func WriteValue(w io.Writer, val Value) error {
	switch val.Type {
	case TypeSimpleString:
		_, err := w.Write([]byte("+" + val.Str + "\r\n"))
		return err
	case TypeError:
		_, err := w.Write([]byte("-" + val.Str + "\r\n"))
		return err
	case TypeInteger:
		_, err := w.Write([]byte(":" + strconv.Itoa(val.Num) + "\r\n"))
		return err
	case TypeBulkString:
		if val.Null {
			_, err := w.Write([]byte("$-1\r\n"))
			return err
		}
		_, err := w.Write([]byte("$" + strconv.Itoa(len(val.Str)) + "\r\n" + val.Str + "\r\n"))
		return err
	case TypeArray:
		if val.Null {
			_, err := w.Write([]byte("*-1\r\n"))
			return err
		}
		_, err := w.Write([]byte("*" + strconv.Itoa(len(val.Array)) + "\r\n"))
		if err != nil {
			return err
		}
		for _, item := range val.Array {
			err = WriteValue(w, item)
			if err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("unknown value type for serialization")
	}
}

func IsWriteCommand(val Value) bool {
	if val.Type != TypeArray || len(val.Array) == 0 {
		return false
	}
	if val.Array[0].Type != TypeBulkString && val.Array[0].Type != TypeSimpleString {
		return false
	}
	cmd := strings.ToUpper(val.Array[0].Str)
	switch cmd {
	case "SET", "SETNX", "SETEX", "PSETEX", "MSET", "MSETNX", "APPEND",
		"DEL", "UNLINK", "EXPIRE", "EXPIREAT", "PEXPIRE", "PEXPIREAT",
		"INCR", "DECR", "INCRBY", "DECRBY", "INCRBYFLOAT",
		"HSET", "HMSET", "HSETNX", "HDEL", "HINCRBY", "HINCRBYFLOAT",
		"LPUSH", "RPUSH", "LPUSHX", "RPUSHX", "LPOP", "RPOP", "LSET", "LTRIM", "LINSERT",
		"SADD", "SREM", "SPOP", "SMOVE",
		"ZADD", "ZREM", "ZINCRBY",
		"FLUSHALL", "FLUSHDB":
		return true
	}
	return false
}

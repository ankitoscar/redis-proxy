package backend

import (
	"fmt"
	"net"
	"redis-proxy/parser"
	"strings"
)

type Client struct {
	conn   net.Conn
	parser *parser.Parser
}

func Connect(addr string, username string, password string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	client := &Client{
		conn:   conn,
		parser: parser.NewParser(conn),
	}
	if password != "" {
		var authArgs []parser.Value
		if username != "" {
			authArgs = []parser.Value{
				{Type: parser.TypeBulkString, Str: "AUTH"},
				{Type: parser.TypeBulkString, Str: username},
				{Type: parser.TypeBulkString, Str: password},
			}
		} else {
			authArgs = []parser.Value{
				{Type: parser.TypeBulkString, Str: "AUTH"},
				{Type: parser.TypeBulkString, Str: password},
			}
		}
		authCmd := parser.Value{
			Type:  parser.TypeArray,
			Array: authArgs,
		}
		resp, err := client.Execute(authCmd)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if resp.Type != parser.TypeSimpleString || strings.ToUpper(resp.Str) != "OK" {
			conn.Close()
			return nil, fmt.Errorf("AUTH failed: %s", resp.Str)
		}
	}
	return client, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Execute(cmd parser.Value) (parser.Value, error) {
	err := parser.WriteValue(c.conn, cmd)
	if err != nil {
		return parser.Value{}, err
	}
	return c.parser.Read()
}

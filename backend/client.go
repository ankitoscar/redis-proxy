package backend

import (
	"net"
	"redis-proxy/parser"
)

type Client struct {
	conn   net.Conn
	parser *parser.Parser
}

func Connect(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		parser: parser.NewParser(conn),
	}, nil
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

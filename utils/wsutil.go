package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"

	"nhooyr.io/websocket"
)

type WebsocketBinaryConnToConn struct {
	Conn          *websocket.Conn
	CloseCallback func() error
	buf           *bytes.Buffer
}

func (c *WebsocketBinaryConnToConn) Read(p []byte) (n int, err error) {
	if c.buf == nil {
		c.buf = new(bytes.Buffer)
	}
	if c.buf.Len() < cap(p) {
		_, chunk, err := c.Conn.Read(context.Background())
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
		} else {
			c.buf.Write(chunk)
		}
	}
	return c.buf.Read(p)
}

func (c *WebsocketBinaryConnToConn) Write(p []byte) (n int, err error) {
	err = c.Conn.Write(context.Background(), websocket.MessageBinary, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *WebsocketBinaryConnToConn) Close() error {
	if c.CloseCallback != nil {
		err := c.CloseCallback()
		if err != nil {
			return err
		}
	}
	return c.Conn.Close(websocket.StatusNormalClosure, "Bye")
}

type WebsocketBase64ConnToConn struct {
	Conn          *websocket.Conn
	CloseCallback func() error
	buf           *bytes.Buffer
}

func (c *WebsocketBase64ConnToConn) Read(p []byte) (n int, err error) {
	if c.buf == nil {
		c.buf = new(bytes.Buffer)
	}
	if c.buf.Len() < cap(p) {
		_, chunk, err := c.Conn.Read(context.Background())
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
		} else {
			b, err := base64.StdEncoding.DecodeString(string(chunk))
			if err != nil {
				return 0, err
			}
			c.buf.Write(b)
		}
	}
	return c.buf.Read(p)
}

func (c *WebsocketBase64ConnToConn) Write(p []byte) (n int, err error) {
	msg := []byte(base64.StdEncoding.EncodeToString(p))
	err = c.Conn.Write(context.Background(), websocket.MessageText, msg)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *WebsocketBase64ConnToConn) Close() error {
	if c.CloseCallback != nil {
		err := c.CloseCallback()
		if err != nil {
			return err
		}
	}
	return c.Conn.Close(websocket.StatusNormalClosure, "Bye")
}

func WebSocketConnToConn(conn *websocket.Conn, closeCallback func() error) io.ReadWriteCloser {
	if conn.Subprotocol() == "base64" {
		return &WebsocketBase64ConnToConn{
			Conn:          conn,
			CloseCallback: closeCallback,
		}
	} else {
		return &WebsocketBinaryConnToConn{
			Conn:          conn,
			CloseCallback: closeCallback,
		}
	}
}

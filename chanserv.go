package chanserv

import (
	"net"
	"time"
)

type Frame interface {
	Bytes() []byte
}

type MetaData interface {
	RemoteAddr() string
}

type Source interface {
	Header() []byte
	Meta() MetaData
	Out() <-chan Frame
}

type SourceFunc func(reqBody []byte) <-chan Source

type Multiplexer interface {
	Bind(net, laddr string) (net.Listener, error)
	DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error)
}

type Server interface {
	ListenAndServe(vAddr string, src SourceFunc) error
}

type PostTag int

const (
	MetaTag PostTag = iota
	BucketTag
)

type Client interface {
	LookupAndPost(vAddr string, body []byte, tags map[PostTag]string) (<-chan Source, error)
}

package chanserv

import "net"

type Frame interface {
	Bytes() []byte
}

type Source interface {
	Header() []byte
	Out() <-chan Frame
}

type SourceFunc func(reqBody []byte) <-chan Source

type Server interface {
	ListenAndServe(addr string, source SourceFunc) error
	Serve(l net.Listener, source SourceFunc) error
}

type Client interface {
	Post(addr string, body []byte) (<-chan Source, error)
}

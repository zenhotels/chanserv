package chanserv

type Frame interface {
	Bytes() []byte
}

type Source interface {
	Header() []byte
	Out() <-chan Frame
}

type SourceFunc func(reqBody []byte) <-chan Source

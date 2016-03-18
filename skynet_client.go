package chanserv

import (
	"io"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
)

type SkyClient struct {
	DialTimeout  time.Duration
	SourceBuffer int
	FrameBuffer  int
	OnError      func(err error)

	network skyapi.Network
	initCtl sync.Once
}

var defaultClient = &SkyClient{}

func (s *SkyClient) init() {
	s.initCtl.Do(func() {
		if s.DialTimeout == 0 {
			s.DialTimeout = 10 * time.Second
		}
		if s.SourceBuffer == 0 {
			s.SourceBuffer = 32
		}
		if s.FrameBuffer == 0 {
			s.FrameBuffer = 1024
		}
		s.network = skyproto_v1.SkyNetworkNew()
	})
}

type source struct {
	header []byte
	out    chan Frame
}

func (s source) Header() []byte {
	return s.header
}

func (s source) Out() <-chan Frame {
	return s.out
}

type frame []byte

func (f frame) Bytes() []byte {
	return []byte(f)
}

func Post(addr string, body []byte) (<-chan Source, error) {
	return defaultClient.Post(addr, body)
}

func (s *SkyClient) Post(addr string, body []byte) (<-chan Source, error) {
	s.init()

	conn, err := s.network.DialTimeout(addr, "chanserv:1000", s.DialTimeout)
	if err != nil {
		sourceChan := make(chan Source, 1)
		close(sourceChan)
		return sourceChan, err
	}
	if err := writeFrame(conn, body); err != nil {
		sourceChan := make(chan Source, 1)
		close(sourceChan)
		conn.Close()
		return sourceChan, err
	}
	buf, err := readFrame(conn)
	if err != nil {
		sourceChan := make(chan Source, 1)
		close(sourceChan)
		conn.Close()
		if err != io.EOF {
			return sourceChan, err
		}
		return sourceChan, nil
	}
	sourceChan := make(chan Source, s.SourceBuffer)
	go func() {
		expectHeader := true
		out := make(chan Frame, s.FrameBuffer)
		src := source{out: out}
		for err == nil {
			if expectHeader {
				expectHeader = false
				src.header = buf
				buf, err = readFrame(conn)
				continue
			}
			go s.discover(addr, string(buf), out)
			sourceChan <- src

			expectHeader = true
			out = make(chan Frame, s.FrameBuffer)
			src = source{out: out}
			buf, err = readFrame(conn)
		}
		conn.Close()
		close(sourceChan)
		if err != io.EOF {
			s.reportErr(err)
		}
	}()
	return sourceChan, nil
}

func (s *SkyClient) reportErr(err error) bool {
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return true
	}
	return false
}

func (s *SkyClient) discover(network, addr string, out chan<- Frame) {
	defer close(out)

	conn, err := s.network.DialTimeout(network, addr, s.DialTimeout)
	if s.reportErr(err) {
		return
	}
	defer conn.Close()

	buf, err := readFrame(conn)
	for err == nil {
		out <- frame(buf)
		buf, err = readFrame(conn)
	}
	if err != io.EOF {
		s.reportErr(err)
	}
}

package chanserv

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
)

type SkyClient struct {
	DialTimeout    time.Duration
	FrameRTimeout  time.Duration
	MasterRTimeout time.Duration
	MasterWTimeout time.Duration
	SourceBuffer   int
	FrameBuffer    int
	OnError        func(err error)

	ServiceName  string
	ServiceTags  []string
	RegistryAddr string

	initOnce sync.Once
	joinOnce sync.Once
	net      skyapi.Multiplexer
}

var defaultClient = &SkyClient{}

func (s *SkyClient) init() {
	s.initOnce.Do(func() {
		if s.DialTimeout == 0 {
			s.DialTimeout = 10 * time.Second
		}
		if s.SourceBuffer == 0 {
			s.SourceBuffer = 32
		}
		if s.FrameBuffer == 0 {
			s.FrameBuffer = 1024
		}
		s.net = skyapi.SkyNet.WithEnv(s.ServiceTags...)
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
	return defaultClient.DialAndPost(addr, body)
}

func (s *SkyClient) LookupAndPost(body []byte, opt ...Options) (<-chan Source, error) {
	s.init()

	var o Options
	if len(opt) > 0 {
		o = opt[0]
	}
	retErr := func(err error) (<-chan Source, error) {
		sourceChan := make(chan Source)
		close(sourceChan)
		return sourceChan, err
	}
	if len(s.ServiceName) == 0 {
		return retErr(errors.New("no service name provided"))
	} else if len(s.RegistryAddr) == 0 {
		return retErr(errors.New("no registry addr provided"))
	}

	var err error
	s.joinOnce.Do(func() {
		err = s.net.Join("tcp4", s.RegistryAddr)
	})
	if err != nil {
		return retErr(err)
	}

	network := o.RegistryBucket()
	conn, err := s.net.DialTimeout(network, s.ServiceName, s.DialTimeout)
	if err != nil {
		return retErr(err)
	}

	return s.post(s.net, conn, body)
}

type Options struct {
	Bucket string
}

func (o *Options) RegistryBucket() string {
	if len(o.Bucket) == 0 {
		return ""
	}
	return fmt.Sprintf("registry://%s", o.Bucket)
}

func (s *SkyClient) DialAndPost(addr string, body []byte, opt ...Options) (<-chan Source, error) {
	s.init()

	var o Options
	if len(opt) > 0 {
		o = opt[0]
	}
	retErr := func(err error) (<-chan Source, error) {
		sourceChan := make(chan Source)
		close(sourceChan)
		return sourceChan, err
	}

	net := skyapi.SkyNet.New()
	if err := net.Join("tcp4", addr); err != nil {
		return retErr(err)
	}
	network := o.RegistryBucket()
	conn, err := net.DialTimeout(network, "chanserv:1000", s.DialTimeout)
	if err != nil {
		return retErr(err)
	}

	return s.post(net, conn, body)
}

func (s *SkyClient) post(net skyapi.Multiplexer,
	conn net.Conn, body []byte) (<-chan Source, error) {

	if s.MasterWTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(s.MasterWTimeout))
	}
	if err := writeFrame(conn, body); err != nil {
		sourceChan := make(chan Source)
		close(sourceChan)
		conn.Close()
		return sourceChan, err
	}

	if s.MasterRTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(s.MasterRTimeout))
	}
	buf, err := readFrame(conn)
	if err != nil {
		sourceChan := make(chan Source)
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
				if s.MasterRTimeout > 0 {
					conn.SetReadDeadline(time.Now().Add(s.MasterRTimeout))
				}
				buf, err = readFrame(conn)
				continue
			}
			go s.discover(net, string(buf), out)
			sourceChan <- src

			expectHeader = true
			out = make(chan Frame, s.FrameBuffer)
			src = source{out: out}
			if s.MasterRTimeout > 0 {
				conn.SetReadDeadline(time.Now().Add(s.MasterRTimeout))
			}
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

func (s *SkyClient) discover(net skyapi.Multiplexer, addr string, out chan<- Frame) {
	defer close(out)

	conn, err := net.DialTimeout("", addr, s.DialTimeout)
	if s.reportErr(err) {
		return
	}
	defer conn.Close()

	if s.FrameRTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(s.FrameRTimeout))
	}
	buf, err := readFrame(conn)
	for err == nil {
		out <- frame(buf)

		if s.FrameRTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(s.FrameRTimeout))
		}
		buf, err = readFrame(conn)
	}
	if err != io.EOF {
		s.reportErr(err)
	}
}

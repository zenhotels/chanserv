package chanserv

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
	"hotcore.in/skynet/skylab/registry"
)

type SkyClient struct {
	DialTimeout    time.Duration
	FrameRTimeout  time.Duration
	MasterRTimeout time.Duration
	MasterWTimeout time.Duration
	SourceBuffer   int
	FrameBuffer    int
	OnError        func(err error)

	AppName      string
	AppTags      []string
	RegistryAddr string

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
	return defaultClient.DialAndPost(addr, body)
}

func (s *SkyClient) LookupAndPost(body []byte) (<-chan Source, error) {
	s.init()

	retErr := func(err error) (<-chan Source, error) {
		sourceChan := make(chan Source)
		close(sourceChan)
		return sourceChan, err
	}
	if len(s.AppName) == 0 {
		return retErr(errors.New("no app name provided"))
	} else if len(s.RegistryAddr) == 0 {
		return retErr(errors.New("no registry address provided"))
	}
	regNet, err := registry.RegistryNetwork(s.network, s.RegistryAddr, s.AppTags...)
	if err != nil {
		return retErr(err)
	}
	registryEntry := fmt.Sprintf("tcp4://registry/%s", s.AppName)
	conn, err := regNet.DialTimeout(registryEntry, "chanserv", s.DialTimeout)
	if err != nil {
		return retErr(err)
	}

	return s.post(conn, body)
}

func (s *SkyClient) DialAndPost(addr string, body []byte) (<-chan Source, error) {
	s.init()
	conn, err := s.network.DialTimeout(addr, "chanserv:1000", s.DialTimeout)
	if err != nil {
		sourceChan := make(chan Source)
		close(sourceChan)
		return sourceChan, err
	}
	return s.post(conn, body)
}

func (s *SkyClient) post(conn net.Conn, body []byte) (<-chan Source, error) {
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
			go s.discover(conn.RemoteAddr().Network(), string(buf), out)
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

func (s *SkyClient) discover(network, addr string, out chan<- Frame) {
	defer close(out)

	conn, err := s.network.DialTimeout(network, addr, s.DialTimeout)
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

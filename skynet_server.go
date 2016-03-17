package chanserv

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
)

type SkyServer struct {
	Addr        string
	Listener    net.Listener
	Source      SourceFunc
	OnError     func(err error)
	OnChanError func(err error)

	CriticalErrMass   int
	OnCriticalErrMass func(err error)

	MasterWTimeout time.Duration
	ServeTimeout   time.Duration

	chanOffset uint64
	chanMap    map[uint64]skyChannel
	mux        sync.RWMutex
	network    skyapi.Network
	initCtl    sync.Once
}

func (s *SkyServer) init() {
	s.initCtl.Do(func() {
		if s.CriticalErrMass == 0 {
			s.CriticalErrMass = 10
		}
		if s.OnCriticalErrMass == nil {
			s.OnCriticalErrMass = func(err error) {
				time.Sleep(30 * time.Second)
			}
		}
		s.chanOffset = 1000
		s.chanMap = make(map[uint64]skyChannel)
		s.network = skyproto_v1.SkyNetworkNew()
	})
}

func ListenAndServe(addr string, source SourceFunc) error {
	server := &SkyServer{
		Addr:   addr,
		Source: source,
	}
	return server.Serve()
}

func Serve(l net.Listener, source SourceFunc) error {
	if l == nil {
		return errors.New("chanserv: no valid listener provided")
	}
	server := &SkyServer{
		Listener: l,
		Source:   source,
	}
	return server.Serve()
}

func (s *SkyServer) Serve() error {
	s.init()

	var listener net.Listener
	var err error
	if s.Listener != nil {
		listener, err = s.network.Bind(s.Listener, 1000)
	} else {
		listener, err = s.network.BindNet("tcp4", s.Addr, 1000)
		if err == nil {
			defer listener.Close()
		}
	}
	if err != nil {
		return err
	}

	var errMass int
	for {
		masterConn, err := listener.Accept()
		if s.reportErr(err) {
			errMass++
			if s.CriticalErrMass > 0 && errMass >= s.CriticalErrMass {
				s.OnCriticalErrMass(err)
			}
			continue
		}
		errMass = 0
		go s.serveMaster(masterConn, s.Source)
	}
}

func (s *SkyServer) serveMaster(masterConn net.Conn, masterFn SourceFunc) {
	if s.ServeTimeout > 0 {
		masterConn.SetDeadline(time.Now().Add(s.ServeTimeout))
	}
	defer masterConn.Close()

	reqBody, err := readFrame(masterConn)
	if s.reportErr(err) {
		return
	}

	var t *time.Timer
	if s.MasterWTimeout > 0 {
		t = time.NewTimer(s.MasterWTimeout)
	} else {
		t = time.NewTimer(time.Minute)
		t.Stop()
	}

	sourceChan := masterFn(reqBody)
	for {
		select {
		case <-t.C:
			return
		case out, ok := <-sourceChan:
			if !ok {
				// sourcing is over
				return
			}
			port, err := s.bindChannel(out.Out())
			if s.reportErr(err) {
				continue
			}
			addr := []byte(fmt.Sprintf("chanserv:%d", port))
			if !s.reportErr(writeFrame(masterConn, out.Header())) {
				if s.reportErr(writeFrame(masterConn, addr)) {
					continue
				}
			}
			if s.MasterWTimeout > 0 {
				t.Reset(s.MasterWTimeout)
			}
		}
	}
}

func (s *SkyServer) bindChannel(out <-chan Frame) (uint64, error) {
	s.mux.Lock()

	s.chanOffset++
	offset := s.chanOffset

	var listener net.Listener
	var err error
	if s.Listener != nil {
		listener, err = s.network.Bind(s.Listener, offset)
	} else {
		listener, err = s.network.BindNet("tcp4", s.Addr, offset)
		if err == nil {
			defer listener.Close()
		}
	}
	if err != nil {
		s.chanOffset--
		s.mux.Unlock()
		s.reportErr(err)
		return 0, err
	}

	c := skyChannel{
		Listener: listener,
		outChan:  out,
		onError:  s.OnError,
		onClosed: func() {
			s.unbindChannel(offset)
		},
	}
	if s.OnChanError != nil {
		c.onError = s.OnChanError
	}
	go c.serve(s.ServeTimeout)
	s.chanMap[offset] = c
	s.mux.Unlock()
	return offset, nil
}

func (s *SkyServer) unbindChannel(offset uint64) {
	s.mux.Lock()
	if c, ok := s.chanMap[offset]; ok {
		c.outChan = nil
	}
	delete(s.chanMap, offset)
	s.mux.Unlock()
}

func (s *SkyServer) reportErr(err error) bool {
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return true
	}
	return false
}

type skyChannel struct {
	net.Listener

	outChan  <-chan Frame
	onClosed func()
	onError  func(err error)
}

func (c skyChannel) serve(timeout time.Duration) {
	defer c.Close()

	conn, err := c.Accept()
	if err != nil {
		return
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}
	defer conn.Close()
	defer c.onClosed()
	for frame := range c.outChan {
		if err := writeFrame(conn, frame.Bytes()); err != nil {
			c.reportErr(err)
		}
	}
}

func (c skyChannel) reportErr(err error) {
	if c.onError != nil {
		c.onError(err)
	}
}

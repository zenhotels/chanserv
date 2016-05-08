package chanserv

import (
	"errors"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
)

type SkyServer struct {
	Addr        string
	Listener    net.Listener
	Source      SourceFunc
	OnError     func(err error)
	OnChanError func(err error)

	ServiceName  string
	ServiceTags  []string
	RegistryAddr string

	CriticalErrMass   int
	OnCriticalErrMass func(err error)

	FrameWTimeout       time.Duration
	SourceRTimeout      time.Duration
	MasterRTimeout      time.Duration
	MasterWTimeout      time.Duration
	ServeTimeout        time.Duration
	FramesAcceptTimeout time.Duration

	initCtl sync.Once
	net     skyapi.Multiplexer
}

func (s *SkyServer) init() {
	s.initCtl.Do(func() {
		if s.FramesAcceptTimeout == 0 {
			s.FramesAcceptTimeout = 30 * time.Second
		}
		if s.CriticalErrMass == 0 {
			s.CriticalErrMass = 10
		}
		if s.OnCriticalErrMass == nil {
			s.OnCriticalErrMass = func(err error) {
				time.Sleep(30 * time.Second)
			}
		}
		s.net = skyapi.SkyNet.WithEnv(s.ServiceTags...)
	})
}

func JoinAndServe(addr string, source SourceFunc, svcName string, tags ...string) error {
	server := &SkyServer{
		RegistryAddr: addr,

		Source:      source,
		ServiceName: svcName,
		ServiceTags: tags,
	}
	return server.JoinAndServe()
}

func ListenAndServe(addr string, source SourceFunc) error {
	server := &SkyServer{
		Addr:   addr,
		Source: source,
	}
	return server.ListenAndServe()
}

func (s *SkyServer) ListenAndServe() error {
	s.init()

	if s.Listener != nil {
		s.serve(s.Listener)
		return nil
	}
	if err := s.net.ListenAndServe("tcp4", s.Addr); err != nil {
		return err
	}
	if err := s.net.Join("tcp4", s.Addr); err != nil {
		return err
	}
	l, err := s.net.Bind("", "chanserv:1000")
	if err != nil {
		return err
	}
	defer l.Close()

	s.serve(l)
	return nil
}

func (s *SkyServer) JoinAndServe() error {
	s.init()

	if len(s.ServiceName) == 0 {
		return errors.New("no service name provided")
	} else if len(s.RegistryAddr) == 0 {
		return errors.New("no registry address provided")
	}

	if err := s.net.ListenAndServe("tcp4", ":0"); err != nil {
		return err
	}
	if err := s.net.Join("tcp4", s.RegistryAddr); err != nil {
		return err
	}
	l, err := s.net.Bind("", s.ServiceName)
	if err != nil {
		return err
	}
	defer l.Close()

	s.serve(l)
	return nil
}

func (s *SkyServer) serve(listener net.Listener) {
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

	if s.MasterRTimeout > 0 {
		masterConn.SetReadDeadline(time.Now().Add(s.MasterRTimeout))
	}
	reqBody, err := readFrame(masterConn)
	if s.reportErr(err) {
		return
	}

	var t *time.Timer
	if s.SourceRTimeout > 0 {
		t = time.NewTimer(s.SourceRTimeout)
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
			if s.SourceRTimeout > 0 {
				t.Reset(s.SourceRTimeout)
			}
			vAddr, err := s.bindChannel(out.Out())
			if s.reportErr(err) {
				continue
			}
			if s.MasterWTimeout > 0 {
				masterConn.SetWriteDeadline(time.Now().Add(s.MasterWTimeout))
			}
			if !s.reportErr(writeFrame(masterConn, out.Header())) {
				if s.reportErr(writeFrame(masterConn, []byte(vAddr))) {
					continue
				}
			}
		}
	}
}

func (s *SkyServer) bindChannel(out <-chan Frame) (string, error) {
	listener, err := s.net.Bind("", ":0")
	if err != nil {
		s.reportErr(err)
		return "", err
	}
	c := skyChannel{
		Listener: listener,
		outChan:  out,
		onError:  s.OnError,
		// onClosed: func() {
		// 	s.unbindChannel(vAddr)
		// },
		wTimeout: s.FrameWTimeout,
		aTimeout: s.FramesAcceptTimeout,
	}
	if s.OnChanError != nil {
		c.onError = s.OnChanError
	}
	vAddr := listener.Addr().String()
	go c.serve(s.ServeTimeout)
	return vAddr, nil
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
	wTimeout time.Duration
	aTimeout time.Duration
}

func (c skyChannel) serve(timeout time.Duration) {
	defer c.Close()

	conn, err := skyapi.AcceptTimeout(c, c.aTimeout)
	if err != nil {
		return
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}
	defer conn.Close()
	for frame := range c.outChan {
		if c.wTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(c.wTimeout))
		}
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

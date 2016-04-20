package chanserv

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
	"hotcore.in/skynet/skylab/registry"
)

type SkyServer struct {
	Addr        string
	Listener    net.Listener
	Source      SourceFunc
	OnError     func(err error)
	OnChanError func(err error)

	AppName      string
	AppTags      []string
	RegistryHost string
	RegistryPort int

	CriticalErrMass   int
	OnCriticalErrMass func(err error)

	FrameWTimeout       time.Duration
	SourceRTimeout      time.Duration
	MasterRTimeout      time.Duration
	MasterWTimeout      time.Duration
	ServeTimeout        time.Duration
	FramesAcceptTimeout time.Duration

	chanOffset uint64
	chanMap    map[uint64]skyChannel
	mux        sync.RWMutex
	network    skyapi.Network
	initCtl    sync.Once
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
		s.chanOffset = 1000
		s.chanMap = make(map[uint64]skyChannel)
		s.network = skyproto_v1.SkyNetworkNew()
	})
}

func RegistryAndServe(regHost string, regPort int, source SourceFunc, appName string, tags ...string) error {
	server := &SkyServer{
		RegistryHost: regHost,
		RegistryPort: regPort,
		Source:       source,
		AppName:      appName,
		AppTags:      tags,
	}
	return server.RegistryAndServe()
}

func ListenAndServe(addr string, source SourceFunc) error {
	server := &SkyServer{
		Addr:   addr,
		Source: source,
	}
	return server.ListenAndServe()
}

func Serve(l net.Listener, source SourceFunc) error {
	if l == nil {
		return errors.New("chanserv: no valid listener provided")
	}
	server := &SkyServer{
		Source: source,
	}
	return server.serve(l)
}

func (s *SkyServer) ListenAndServe() error {
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

	return s.serve(listener)
}

func (s *SkyServer) RegistryAndServe() error {
	s.init()

	if len(s.AppName) == 0 {
		return errors.New("no app name provided")
	} else if len(s.RegistryHost) == 0 {
		return errors.New("no registry host provided")
	} else if s.RegistryPort <= 0 {
		return errors.New("no registry port provided")
	}

	tcpAddr := fmt.Sprintf("%s:%d", s.RegistryHost, s.RegistryPort)
	skyAddr := fmt.Sprintf("skynet:%d", s.RegistryPort)
	regAddr, err := s.network.MakeAddr(tcpAddr, skyAddr)
	if err != nil {
		return err
	}
	regNet, err := registry.RegistryNetwork(s.network, regAddr, s.AppTags...)
	if err != nil {
		return err
	}

	var listener net.Listener
	if s.Listener != nil {
		listener, err = regNet.Bind(s.Listener, 1000)
	} else {
		networkAddr := fmt.Sprintf("tcp4://registry/%s", s.AppName)
		listener, err = regNet.BindNet(networkAddr, s.Addr, 1000)
		if err == nil {
			defer listener.Close()
		}
	}
	if err != nil {
		return err
	}

	return s.serve(listener)
}

func (s *SkyServer) serve(listener net.Listener) error {
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
			port, err := s.bindChannel(out.Out())
			if s.reportErr(err) {
				continue
			}
			if s.MasterWTimeout > 0 {
				masterConn.SetWriteDeadline(time.Now().Add(s.MasterWTimeout))
			}
			addr := []byte(fmt.Sprintf("chanserv:%d", port))
			if !s.reportErr(writeFrame(masterConn, out.Header())) {
				if s.reportErr(writeFrame(masterConn, addr)) {
					continue
				}
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
		wTimeout: s.FrameWTimeout,
		aTimeout: s.FramesAcceptTimeout,
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
	defer c.onClosed()
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

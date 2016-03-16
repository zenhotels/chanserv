package chanserv

import (
	"log"
	"net"
	"sync"
	"time"

	"hotcore.in/skynet/skyapi/skyproto_v1"
)

type SkyServer struct {
	Addr        string
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
		s.chanMap = make(map[uint64]skyChannel)
	})
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
	log.Println("init done")
	tcp, err := net.Listen("tcp4", s.Addr)
	if err != nil {
		return err
	}

	log.Println("listen at", s.Addr)
	var errMass int
	tcpConn, err := tcp.Accept()
	log.Println("accepted tcp", tcpConn.LocalAddr(), tcpConn.RemoteAddr(), "error:", err)
TCPLoop:
	for {
		if s.reportErr(err) {
			errMass++
			if s.CriticalErrMass > 0 && errMass >= s.CriticalErrMass {
				s.OnCriticalErrMass(err)
			}
			tcpConn, err = tcp.Accept()
			continue
		}
		log.Println("new sky network over TCP")
		skynet := skyproto_v1.SkyNetworkNew(tcpConn)
		log.Println("listen skynet over TCP", s.Addr, "for skynet:12123123")
		sky, err := skynet.Listen("", "skynet:12123123")
		log.Println("listening. skynet error:", err)
		if s.reportErr(err) {
			tcpConn.Close()
			// reset the backing TCP connection
			tcpConn, err = tcp.Accept()
			continue
		}
		// HACK test
		// time.Sleep(time.Second)
		log.Println("handling incoming master requests at", sky.Addr())
		// handle incoming master requests
		masterConn, err := sky.Accept()
		log.Println("ACCEPTED master conn, err:", err)

		for {
			if s.reportErr(err) {
				tcpConn.Close()
				// reset the backing TCP connection
				tcpConn, err = tcp.Accept()
				continue TCPLoop
			}
			errMass = 0
			log.Println("serving master", masterConn.LocalAddr(), masterConn.RemoteAddr())
			go s.serveMaster(tcpConn, masterConn, s.Source)
			return nil
			// masterConn, err = sky.Accept()
		}
	}
}

func (s *SkyServer) serveMaster(tcpConn net.Conn, masterConn net.Conn, masterFn SourceFunc) {
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
			addr, err := s.bindChannel(tcpConn, out.Out())
			if s.reportErr(err) {
				continue
			}
			if !s.reportErr(writeFrame(out.Header(), masterConn)) {
				if s.reportErr(writeFrame([]byte("skynet"+addr), masterConn)) {
					continue
				}
			}
			if s.MasterWTimeout > 0 {
				t.Reset(s.MasterWTimeout)
			}
		}
	}
}

func (s *SkyServer) bindChannel(tcpConn net.Conn, out <-chan Frame) (string, error) {
	s.mux.Lock()

	s.chanOffset++
	offset := s.chanOffset
	addr := chanAddr(offset)
	log.Println("binding chan", addr)
	skynet := skyproto_v1.SkyNetworkNew(tcpConn)
	l, err := skynet.Listen("", addr)
	log.Println("listening for skynet on", addr, tcpConn.LocalAddr(), "error:", err)
	if err != nil {
		s.chanOffset--
		s.mux.Unlock()
		s.reportErr(err)
		return "", err
	}
	c := skyChannel{
		Listener: l,
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
	return addr, nil
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
	if s.OnError != nil {
		s.OnError(err)
	}
	if err != nil {
		panic(err.Error())
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
	log.Println("accepting in chan")
	conn, err := c.Listener.Accept()
	log.Println("ACCEPTED chan conn, error:", err)
	if err != nil {
		return
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}
	defer conn.Close()
	defer c.onClosed()
	for frame := range c.outChan {
		if err := writeFrame(frame.Bytes(), conn); err != nil {
			c.reportErr(err)
		}
	}
}

func (c skyChannel) reportErr(err error) {
	if c.onError != nil {
		c.onError(err)
	}
}

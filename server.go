package chanserv

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type server struct {
	mpx Multiplexer

	maxErrMass   int
	onMaxErrMass func(mass int, err error)
	onMpxError   func(err error)
	onError      func(err error)
	onChanError  func(err error)

	timeouts       serverTimeouts
	useCompression bool
}

type serverTimeouts struct {
	servingTimeout    time.Duration
	sourcingTimeout   time.Duration
	chanAcceptTimeout time.Duration

	masterReadTimeout  time.Duration
	masterWriteTimeout time.Duration
	frameWriteTimeout  time.Duration
}

// NewServer initializes a new server using the provided multiplexer for
// the network capabilities. Refer to the server options if you want to specify
// timeouts and error callbacks.
func NewServer(mpx Multiplexer, opts ...ServerOption) Server {
	srv := server{
		mpx: mpx,

		onError:     func(err error) {}, // no report
		onChanError: func(err error) {}, // no report
		onMaxErrMass: func(mass int, err error) {
			// TODO: graceful fallback based on the mass value
			time.Sleep(30 * time.Second)
		},

		maxErrMass:     10,
		useCompression: false,
		timeouts: serverTimeouts{
			// chanAcceptTimeout triggers when a channel has been announced
			// but nobody has discovered it in time.
			chanAcceptTimeout: 30 * time.Second,
		},
	}
	for _, o := range opts {
		o(&srv)
	}
	return srv
}

func (s server) ListenAndServe(vAddr string, srcFn SourceFunc) error {
	if s.mpx == nil {
		return errors.New("chanserv: mpx not set")
	}
	l, err := s.mpx.Bind("", vAddr)
	if err != nil {
		err = fmt.Errorf("chanserv mpx.Bind: %v", err)
		return err
	}
	// warn: serve will close listener
	go s.serve(l, srcFn)
	return nil
}

func (s server) serve(listener net.Listener, srcFn SourceFunc) {
	defer listener.Close()

	var errMass int
	for {
		masterConn, err := listener.Accept()
		if err != nil {
			err = fmt.Errorf("master listener.Accept: %v", err)
			s.onError(err)
			errMass++
			if s.maxErrMass > 0 && errMass >= s.maxErrMass {
				s.onMaxErrMass(errMass, err)
			}
			continue
		}
		errMass = 0
		go s.serveMaster(masterConn, srcFn)
	}
}

func (s server) serveMaster(masterConn net.Conn, srcFn SourceFunc) {
	if s.timeouts.servingTimeout > 0 {
		masterConn.SetDeadline(time.Now().Add(s.timeouts.servingTimeout))
	}
	defer masterConn.Close()

	if d := s.timeouts.masterReadTimeout; d > 0 {
		masterConn.SetReadDeadline(time.Now().Add(d))
	}
	reqBody, err := readFrame(masterConn, false)
	if err != nil {
		err = fmt.Errorf("master readFrame: %v", err)
		s.onError(err)
		return
	}

	timeout := timerPool.Get().(*time.Timer)
	defer func() {
		timeout.Stop()
		timerPool.Put(timeout)
	}()
	if d := s.timeouts.sourcingTimeout; d > 0 {
		timeout.Reset(d)
	}
	sourceChan := srcFn(reqBody)
	for {
		select {
		case <-timeout.C:
			return
		case out, ok := <-sourceChan:
			if !ok {
				// sourcing is over
				return
			}
			if s.timeouts.sourcingTimeout > 0 {
				timeout.Reset(s.timeouts.sourcingTimeout)
			}
			chanAddr, err := s.bindChannel(out.Out())
			if err != nil {
				err = fmt.Errorf("master bindChannel: %v", err)
				s.onError(err)
				continue
			}
			if d := s.timeouts.masterWriteTimeout; d > 0 {
				masterConn.SetWriteDeadline(time.Now().Add(d))
			}
			if err := writeFrame(masterConn, out.Header()); err == nil {
				if err = writeFrame(masterConn, []byte(chanAddr)); err != nil {
					err = fmt.Errorf("master writeFrame: %v", err)
					s.onError(err)
				}
			} else {
				err = fmt.Errorf("master writeFrame: %v", err)
				s.onError(err)
			}
		}
	}
}

func (s server) bindChannel(out <-chan Frame) (string, error) {
	l, err := s.mpx.Bind("", ":0") // random port
	if err != nil {
		err = fmt.Errorf("bindChannel mpx.Bind: %v", err)
		s.onError(err)
		return "", err
	}
	c := channel{
		Listener: l,
		outChan:  out,
		onError:  s.onChanError,
		wTimeout: s.timeouts.frameWriteTimeout,
		aTimeout: s.timeouts.chanAcceptTimeout,

		useCompression: s.useCompression,
	}
	vAddr := l.Addr().String()
	go c.serve(s.timeouts.servingTimeout)
	return vAddr, nil
}

type channel struct {
	net.Listener

	outChan  <-chan Frame
	wTimeout time.Duration
	aTimeout time.Duration

	onError        func(err error)
	useCompression bool
}

func (c channel) serve(timeout time.Duration) {
	defer c.Close()

	conn, err := acceptTimeout(c, c.aTimeout)
	if err != nil {
		err = fmt.Errorf("channel acceptTimeout: %v", err)
		c.onError(err)
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
		data := frame.Bytes()
		if c.useCompression && needCompression(data) {
			if err := writeCompressedFrame(conn, data); err != nil {
				err = fmt.Errorf("channel writeCompressedFrame: %v", err)
				c.onError(err)
			}
			continue
		}
		if err := writeFrame(conn, data); err != nil {
			err = fmt.Errorf("channel writeFrame: %v", err)
			c.onError(err)
		}
	}
}

package chanserv

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type client struct {
	mpx     Multiplexer
	onError func(err error)

	srcBufSize   int
	frameBufSize int
	timeouts     clientTimeouts
}

type clientTimeouts struct {
	dialTimeout        time.Duration
	masterReadTimeout  time.Duration
	masterWriteTimeout time.Duration
	frameReadTimeout   time.Duration
}

// NewClient initializes a new client using the provided multiplexer for
// the network capabilities. Refer to the client options if you want to specify
// timeouts and error callbacks.
func NewClient(mpx Multiplexer, opts ...ClientOption) Client {
	cli := client{
		mpx:     mpx,
		onError: func(err error) {}, // no report

		srcBufSize:   128,
		frameBufSize: 1024,
		timeouts: clientTimeouts{
			dialTimeout: 10 * time.Second,
		},
	}
	for _, o := range opts {
		o(&cli)
	}
	return cli
}

type source struct {
	header   []byte
	metaData metaData
	out      chan Frame
}

func (s source) Header() []byte {
	return s.header
}

func (s source) Meta() MetaData {
	return s.metaData
}

func (s source) Out() <-chan Frame {
	return s.out
}

type metaData struct {
	localAddr     string
	localNetwork  string
	remoteAddr    string
	remoteNetwork string
}

func (m metaData) RemoteAddr() string {
	return m.remoteAddr
}

func (m metaData) RemoteNetwork() string {
	return m.remoteNetwork
}

func (m metaData) LocalAddr() string {
	return m.localAddr
}

func (m metaData) LocalNetwork() string {
	return m.localNetwork
}

type frame []byte

func (f frame) Bytes() []byte {
	return []byte(f)
}

func (c client) LookupAndPost(vAddr string,
	body []byte, tags map[RequestTag]string) (<-chan Source, error) {

	retErr := func(err error) (<-chan Source, error) {
		srcChan := make(chan Source)
		close(srcChan)
		return srcChan, err
	}
	if c.mpx == nil {
		return retErr(errors.New("chanserv: mpx not set"))
	}
	var svcBucket string // TODO: handle other tags
	if bucket, ok := tags[TagBucket]; ok && len(bucket) > 0 {
		svcBucket = fmt.Sprintf("registry://%s", bucket)
	}
	conn, err := c.mpx.DialTimeout(svcBucket, vAddr, c.timeouts.dialTimeout)
	if err != nil {
		return retErr(err)
	}

	return c.post(conn, body)
}

func (c client) post(conn net.Conn, body []byte) (<-chan Source, error) {
	if d := c.timeouts.masterWriteTimeout; d > 0 {
		conn.SetWriteDeadline(time.Now().Add(d))
	}
	retErr := func(err error) (<-chan Source, error) {
		srcChan := make(chan Source)
		close(srcChan)
		return srcChan, err
	}
	if err := writeFrame(conn, body); err != nil {
		err = fmt.Errorf("post writeFrame: %v", err)
		return retErr(err)
	}

	if d := c.timeouts.masterReadTimeout; d > 0 {
		conn.SetReadDeadline(time.Now().Add(d))
	}
	buf, err := readFrame(conn, false)
	if err != nil {
		if err != io.EOF {
			err = fmt.Errorf("post readFrame: %v", err)
			return retErr(err)
		}
		return retErr(nil)
	}
	srcChan := make(chan Source, c.srcBufSize)

	go func() {
		expectHeader := true
		outChan := make(chan Frame, c.frameBufSize)
		meta := metaData{
			remoteAddr:    conn.RemoteAddr().String(),
			remoteNetwork: conn.RemoteAddr().Network(),
			localAddr:     conn.LocalAddr().String(),
			localNetwork:  conn.LocalAddr().Network(),
		}
		src := source{
			out:      outChan,
			metaData: meta,
		}
		for err == nil {
			if expectHeader {
				src.header = buf
				expectHeader = false
				if d := c.timeouts.masterReadTimeout; d > 0 {
					conn.SetReadDeadline(time.Now().Add(d))
				}
				buf, err = readFrame(conn, false)
				continue
			}
			vAddr := string(buf)
			go c.discover(vAddr, outChan)
			srcChan <- src

			expectHeader = true
			outChan = make(chan Frame, c.frameBufSize)
			src = source{
				out:      outChan,
				metaData: meta,
			}
			if d := c.timeouts.masterReadTimeout; d > 0 {
				conn.SetReadDeadline(time.Now().Add(d))
			}
			buf, err = readFrame(conn, false)
		}
		conn.Close()
		close(srcChan)
		if err != io.EOF {
			err = fmt.Errorf("post readFrame: %v", err)
			c.onError(err)
		}
	}()

	return srcChan, nil
}

func (c client) discover(vAddr string, out chan<- Frame) {
	defer close(out)

	// channel discover currently uses no additional options
	conn, err := c.mpx.DialTimeout("", vAddr, c.timeouts.dialTimeout)
	if err != nil {
		err = fmt.Errorf("discover mpx.DialTimeout: %v", err)
		c.onError(err)
		return
	}
	defer conn.Close()

	if d := c.timeouts.frameReadTimeout; d > 0 {
		conn.SetReadDeadline(time.Now().Add(d))
	}
	buf, err := readFrame(conn, true)
	for err == nil {
		out <- frame(buf)

		if d := c.timeouts.frameReadTimeout; d > 0 {
			conn.SetReadDeadline(time.Now().Add(d))
		}
		buf, err = readFrame(conn, true)
	}
	if err != io.EOF {
		err = fmt.Errorf("discover readFrame: %v", err)
		c.onError(err)
	}
}

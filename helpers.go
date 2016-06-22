package chanserv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var ErrWrongSize = errors.New("wrong frame size")

// FrameSizeLimit specifies the maximum size of payload in frame,
// this limit may be increased or lifted in future.
const FrameSizeLimit = 100 * 1024 * 1024

func writeFrame(wr io.Writer, frame []byte) (err error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(len(frame)))
	if _, err = wr.Write(buf); err != nil {
		return
	}
	_, err = io.Copy(wr, bytes.NewReader(frame))
	return
}

func readFrame(r io.Reader) ([]byte, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	frameSize := binary.LittleEndian.Uint64(buf)
	// check frame size for bounds
	if frameSize > FrameSizeLimit {
		return nil, ErrWrongSize
	}
	framebuf := bytes.NewBuffer(make([]byte, 0, frameSize))
	_, err := io.CopyN(framebuf, r, int64(frameSize))
	return framebuf.Bytes(), err
}

var timerPool sync.Pool

func init() {
	timerPool.New = func() interface{} {
		t := time.NewTimer(time.Minute)
		t.Stop()
		return t
	}
}

func acceptTimeout(l net.Listener, d time.Duration) (conn net.Conn, err error) {
	timeout := timerPool.Get().(*time.Timer)
	timeout.Reset(d)
	defer func() {
		timeout.Stop()
		timerPool.Put(timeout)
	}()
	done := make(chan struct{})
	go func() {
		conn, err = l.Accept()
		close(done)
	}()
	select {
	case <-done:
	case <-timeout.C:
		err = io.ErrNoProgress
		l.Close()
	}
	return
}

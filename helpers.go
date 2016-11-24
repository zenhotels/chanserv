package chanserv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pierrec/lz4"
)

var ErrWrongSize = errors.New("wrong frame size")
var ErrWrongUncompressedSize = errors.New("wrong uncompressed frame size")

// FrameSizeLimit specifies the maximum size of payload in a frame,
// this limit may be increased or lifted in future.
const FrameSizeLimit = 100 * 1024 * 1024

var CompressionHeader = []byte("lz4!")

func writeFrame(wr io.Writer, frame []byte) (err error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(len(frame)))
	if _, err = wr.Write(buf); err != nil {
		return
	}
	_, err = io.Copy(wr, bytes.NewReader(frame))
	return
}

func writeCompressedFrame(wr io.Writer, frame []byte) (err error) {
	comp := make([]byte, lz4.CompressBlockBound(len(frame)))
	size, err := lz4.CompressBlock(frame, comp, 0)
	if err != nil {
		return err
	}
	comp = comp[:size]
	frameSize := size + len(CompressionHeader) + 8
	buf := make([]byte, 8+len(CompressionHeader)+8)

	binary.LittleEndian.PutUint64(buf, uint64(frameSize))
	copy(buf[8:], CompressionHeader)
	binary.LittleEndian.PutUint64(buf[12:], uint64(len(frame)))
	if _, err = wr.Write(buf); err != nil {
		return
	}
	_, err = io.Copy(wr, bytes.NewReader(comp))
	return
}

func readFrame(r io.Reader, expectCompression bool) ([]byte, error) {
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
	if _, err := io.CopyN(framebuf, r, int64(frameSize)); err != nil {
		return nil, err
	}
	if !expectCompression {
		return framebuf.Bytes(), nil
	}

	data := framebuf.Bytes()
	if len(data) <= len(CompressionHeader)+8 {
		// could not be a compressed frame
		return data, nil
	}
	if !bytes.Equal(CompressionHeader, data[:4]) {
		// doesn't have a compression header
		return data, nil
	}
	uncompressedSize := binary.LittleEndian.Uint64(data[4:])
	// check the size for bounds
	if uncompressedSize > FrameSizeLimit*2 {
		return nil, ErrWrongUncompressedSize
	}
	uncompressed := make([]byte, uncompressedSize)
	size, err := lz4.UncompressBlock(data[12:], uncompressed, 0)
	if err != nil {
		return nil, err
	}
	uncompressed = uncompressed[:size]
	return uncompressed, nil
}

func needCompression(data []byte) bool {
	return len(data) > len(CompressionHeader)+8
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

package chanserv

import (
	"encoding/binary"
	"fmt"
	"io"
)

func writeFrame(frame []byte, w io.Writer) (err error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(len(frame)))
	if _, err = w.Write(buf); err != nil {
		return
	}
	_, err = w.Write(frame)
	return
}

func readFrame(r io.Reader) ([]byte, error) {
	buf := make([]byte, 8)
	if _, err := r.Read(buf); err != nil {
		return nil, err
	}
	v := binary.LittleEndian.Uint64(buf)
	frame := make([]byte, v)
	n, err := r.Read(frame)
	return frame[:n], err
}

func tcpAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

func chanAddr(offset uint64) string {
	return fmt.Sprintf(":%d", offset)
}

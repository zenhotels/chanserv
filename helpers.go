package chanserv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

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
	if _, err := r.Read(buf); err != nil {
		return nil, err
	}
	v := binary.LittleEndian.Uint64(buf)
	frame := bytes.NewBuffer(make([]byte, 0, v))
	_, err := io.CopyN(frame, r, int64(v))
	return frame.Bytes(), err
}

func chanAddr(offset uint64) string {
	return fmt.Sprintf(":%d", offset)
}

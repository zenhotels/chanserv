package chanserv

import (
	"bytes"
	"fmt"
	"log"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestAll(t *testing.T) {
	go func() {
		if err := ListenAndServe(":14000", SourceFn); err != nil {
			log.Fatalln(err)
		}
		log.Println("server died")
	}()

	log.Println("will connect in 3 sec...")
	time.Sleep(3 * time.Second)

	if err := Connect("127.0.0.1:14000"); err != nil {
		log.Fatalln("[ERR]:", err)
	}

	<-time.Tick(5 * time.Minute)
}

func SourceFn(req []byte) <-chan Source {
	log.Println("calling source func for master")
	out := make(chan Source, 1024)
	for i := 0; i < 5; i++ {
		src := source{n: i, data: req}
		out <- src.Run(time.Second*time.Duration(i+1) + 3)
	}
	close(out)
	return out
}

type source struct {
	n      int
	data   []byte
	frames <-chan Frame
}

type frame []byte

func (f frame) Bytes() []byte {
	return f
}

func (s *source) Run(d time.Duration) *source {
	frames := make(chan Frame, 1024)
	go func() {
		frames <- frame([]byte("wait for me!"))
		time.Sleep(d)
		frames <- frame([]byte("ok I'm ready"))
		close(frames)
	}()
	s.frames = frames
	return s
}

func (p *source) Header() []byte {
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "[HEAD] source #%d, req len: %d", p.n, len(p.data))
	return buf.Bytes()
}

func (s *source) Out() <-chan Frame {
	return s.frames
}

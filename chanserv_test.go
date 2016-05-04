package chanserv

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestChanserv(t *testing.T) {
	go func() {
		if err := ListenAndServe(":5555", SourceFn); err != nil {
			log.Fatalln(err)
		}
	}()

	cli := SkyClient{
		DialTimeout: 2 * time.Second,
		OnError: func(err error) {
			t.Fatal("[ERR]", err)
		},
	}
	sources, err := cli.DialAndPost("localhost:5555", []byte("hello"), Options{
		Bucket: "deadbeef",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkSources(t, sources)
}

var (
	registryAddr = "route.hotcore.in:10000"
	testName     = "chanserv_test"
	testEnv      = "local"
)

func init() {
	if host, err := os.Hostname(); err == nil {
		testEnv = host
	}
}

func TestChanservRegistry(t *testing.T) {
	go func() {
		log.Println("Registering", testName, "on", registryAddr, "under env", testEnv)
		if err := JoinAndServe(registryAddr, SourceFn, testName, testEnv); err != nil {
			log.Fatalln(err)
		}
	}()

	cli := SkyClient{
		AppName:      testName,
		AppTags:      []string{testEnv},
		RegistryAddr: registryAddr,
		DialTimeout:  2 * time.Second,
		OnError: func(err error) {
			t.Fatal("[ERR]", err)
		},
	}
	sources, err := cli.LookupAndPost([]byte("hello"), Options{
		Bucket: "deadbeef",
	})
	if err != nil {
		log.Fatalln(err)
	}
	checkSources(t, sources)
}

func checkSources(t *testing.T, sources <-chan Source) {
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		wg.Add(10 * 2)
		var srcs int
		for src := range sources {
			srcs++
			log.Println("[HEAD]", string(src.Header()))
			go func(srcs int, src Source) {
				var frames int
				for frame := range src.Out() {
					log.Printf("[FRAME %d from @%d] %d", frames, srcs, len(frame.Bytes()))
					frames++
					wg.Done()
				}
				log.Println("CLOSED", srcs)
			}(srcs, src)
		}
		wg.Wait()
		close(done)
	}()

	start := time.Now()
	select {
	case <-time.Tick(300 * time.Second):
		t.Fatal("timeout")
	case <-done:
		log.Println("done in", time.Since(start))
	}
}

func SourceFn(req []byte) <-chan Source {
	out := make(chan Source, 10)
	for i := 0; i < 10; i++ {
		src := testSource{n: i + 1, data: req}
		out <- src.Run(time.Second*time.Duration(i+1) + 3)
	}
	close(out)
	return out
}

type testSource struct {
	n      int
	data   []byte
	frames <-chan Frame
}

func (s *testSource) Run(d time.Duration) *testSource {
	frames := make(chan Frame, 2)
	go func() {
		frames <- frame([]byte(strings.Repeat("A", 99999)))
		time.Sleep(300 * time.Millisecond)
		frames <- frame([]byte(strings.Repeat("B", 180200)))
		close(frames)
	}()
	s.frames = frames
	return s
}

func (p *testSource) Header() []byte {
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "source @%d, for request: %s", p.n, p.data)
	return buf.Bytes()
}

func (s *testSource) Out() <-chan Frame {
	return s.frames
}

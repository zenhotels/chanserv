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

	"github.com/zenhotels/astranet"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func srcFn(req []byte) <-chan Source {
	out := make(chan Source, 10)
	for i := 0; i < 10; i++ {
		src := testSource{n: i + 1, data: req}
		out <- src.Run(time.Millisecond*time.Duration(100*i) + 100)
	}
	close(out)
	return out
}

func TestChanserv(t *testing.T) {
	// init the astranet server's Multiplexer
	mpx := astranet.New().Server()
	if err := mpx.ListenAndServe("tcp4", ":5555"); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}

	// start the Server
	srv := NewServer(mpx, ServerOnError(func(err error) {
		log.Println("[server WARN]", err)
	}), ServerUseCompression(true))
	if err := srv.ListenAndServe("chanserv", srcFn); err != nil {
		log.Fatalln("[server ERR]", err)
	}

	// init the astranet client's Multiplexer
	mpx2 := astranet.New().Client()

	// join Multiplexer to the local astranet server
	if err := mpx2.Join("tcp4", "localhost:5555"); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}

	// init the Client
	cli := NewClient(mpx2,
		ClientDialTimeout(5*time.Second),
		ClientOnError(func(err error) {
			log.Println("[client WARN]", err)
		}),
	)

	// lookup the chanserv service and post the request
	sources, err := cli.LookupAndPost("chanserv", []byte("hello"), nil)
	if err != nil {
		log.Fatalln("[client ERR]", err)
	}

	checkSources(t, sources)
}

var (
	registryAddr = "join.igw.io:10000"
	testService  = "chanserv"
	testEnv      = "local"
)

func init() {
	if host, err := os.Hostname(); err == nil {
		testEnv = host
	}
}

func TestRegistryChanserv(t *testing.T) {
	// init the astranet server's Multiplexer
	mpx := astranet.New().Server().WithEnv(testEnv)
	if err := mpx.ListenAndServe("tcp4", "0.0.0.0:0"); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}

	// start the Server
	srv := NewServer(mpx, ServerOnError(func(err error) {
		log.Println("[server WARN]", err)
	}), ServerUseCompression(true))
	if err := srv.ListenAndServe(testService, srcFn); err != nil {
		log.Fatalln("[server ERR]", err)
	}

	// register this chanserv service
	if err := mpx.Join("tcp4", registryAddr); err != nil {
		log.Fatalln("[astranet ERR]", err)
	} else {
		log.Println("[astranet INFO] registered", testService, "on", registryAddr, "with env", testEnv)
	}

	// init the astranet client's Multiplexer
	mpx2 := astranet.New().Client().WithEnv(testEnv)

	// join the discovery service
	if err := mpx2.Join("tcp4", registryAddr); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}

	// init the Client
	cli := NewClient(mpx2,
		// dial timeout includes service discovery time
		ClientDialTimeout(30*time.Second),
		ClientOnError(func(err error) {
			log.Println("[client WARN]", err)
		}),
	)

	// lookup the chanserv service upon registry and post the request
	sources, err := cli.LookupAndPost(testService, []byte("hello"), nil)
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
					if len(frame.Bytes()) > 64 {
						log.Printf("[FRAME %d from @%d] len: %d", frames, srcs, len(frame.Bytes()))
					} else {
						log.Printf("[FRAME %d from @%d] %s", frames, srcs, frame.Bytes())
					}
					frames++
					wg.Done()
				}
				log.Printf("[CLOSED @%d]", srcs)
			}(srcs, src)
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-time.Tick(20 * time.Second):
		t.Fatal("test timeout (waitgroup deadlock?)")
	case <-done:
	}
}

type testSource struct {
	n      int
	data   []byte
	frames <-chan Frame
}

func (s *testSource) Run(d time.Duration) *testSource {
	frames := make(chan Frame, 2)
	go func() {
		frames <- frame([]byte("wait for me!"))
		time.Sleep(d)
		frames <- frame([]byte(strings.Repeat("B", 180*1024)))
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

func (t *testSource) Meta() MetaData {
	return nil
}

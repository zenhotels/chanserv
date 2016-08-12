package chanserv

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/zenhotels/astranet"
)

var chanservCli Client

func init() {
	chanservCli, _ = getChanservAstranet()
	time.Sleep(time.Second)
}

const GB = 1024 * 1024 * 1024
const MB = 1024 * 1024

const (
	heavySourceLimit = 10
	heavyFrameLimit  = 500
	heavyFrameSize   = 1024 * 1024 // 1 MB

	floodSourceLimit = 1000
	floodFrameLimit  = 5000
	floodFrameSize   = 64 // 64 bytes
)

func getMeters() (frameMeter metrics.Meter, frameDataMeter metrics.Meter, stop func()) {
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	stop = func() {
		close(stopC)
		<-doneC
	}
	frameMeter = metrics.NewMeter()
	frameDataMeter = metrics.NewMeter()
	go func() {
		t := time.NewTicker(time.Second)
		var prevFrames int64
		var prevFramesData int64
		for range t.C {
			var mem runtime.MemStats
			snap := frameMeter.Snapshot()
			runtime.ReadMemStats(&mem)
			fmt.Printf("METRICS> Heap: %.1fM\n", float64(mem.HeapInuse)/MB)
			fmt.Printf("METRICS> Goroutines: %.1fK\n", float64(runtime.NumGoroutine())/1000)
			fmt.Printf("METRICS> Frames: %.1fK\n", float64(snap.Count())/1000)
			fmt.Printf("METRICS> Frames/s: %.1fK\n", float64(snap.Count()-prevFrames)/1000)
			prevFrames = snap.Count()

			snap = frameDataMeter.Snapshot()
			fmt.Printf("METRICS> GB: %.3f\n", float64(snap.Count())/GB)
			fmt.Printf("METRICS> Gbit/second: %.3f\n", float64(snap.Count()-prevFramesData)*8/GB)
			prevFramesData = snap.Count()

			fmt.Println()
			select {
			case <-stopC:
				close(doneC)
				t.Stop()
				return
			default:
			}
		}
	}()
	return frameMeter, frameDataMeter, stop
}

func BenchmarkConnectChanserv(b *testing.B) {
	b.ReportAllocs()
	b.SetParallelism(runtime.NumCPU())

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sources, err := chanservCli.LookupAndPost("chanserv-astranet-empty", []byte("hello"), nil)
			if err != nil {
				b.Fatal(err)
				return
			}

			var frames int64
			wg := new(sync.WaitGroup)
			wg.Add(1)
			for src := range sources {
				go func(src Source) {
					for range src.Out() {
						atomic.AddInt64(&frames, 1)
					}
					wg.Done()
				}(src)
			}
			wg.Wait()

			if atomic.LoadInt64(&frames) != 1 {
				b.Fatal("BenchmarkConnectChanserv: frames != 1")
			}
		}
	})
}

func BenchmarkHeavyChanserv(b *testing.B) {
	frameMeter, frameDataMeter, stopFunc := getMeters()
	defer stopFunc()

	for i := 0; i < b.N; i++ {
		sources, err := chanservCli.LookupAndPost("chanserv-astranet-heavy", nil, nil)
		if err != nil {
			b.Fatal(err)
			return
		}

		var frames int64
		wg := new(sync.WaitGroup)
		wg.Add(heavySourceLimit)
		for src := range sources {
			go func(src Source) {
				for frame := range src.Out() {
					frameMeter.Mark(1)
					frameDataMeter.Mark(int64(len(frame.Bytes())))
					atomic.AddInt64(&frames, 1)
				}
				wg.Done()
				// log.Println("source done", atomic.LoadInt64(&frames))
			}(src)
		}
		wg.Wait()

		if atomic.LoadInt64(&frames) != heavySourceLimit*heavyFrameLimit {
			b.Fatalf("BenchmarkHeavyChanserv: frames != %d", heavySourceLimit*heavyFrameLimit)
		}
	}
}

func BenchmarkFloodChanserv(b *testing.B) {
	frameMeter, frameDataMeter, stopFunc := getMeters()
	defer stopFunc()

	for i := 0; i < b.N; i++ {
		sources, err := chanservCli.LookupAndPost("chanserv-astranet-flood", nil, nil)
		if err != nil {
			b.Fatal(err)
			return
		}

		var frames int64
		wg := new(sync.WaitGroup)
		wg.Add(floodSourceLimit)
		for src := range sources {
			go func(src Source) {
				for frame := range src.Out() {
					frameMeter.Mark(1)
					frameDataMeter.Mark(int64(len(frame.Bytes())))
					atomic.AddInt64(&frames, 1)
				}
				wg.Done()
				// log.Println("source done", atomic.LoadInt64(&frames))
			}(src)
		}
		wg.Wait()

		if atomic.LoadInt64(&frames) != floodSourceLimit*floodFrameLimit {
			b.Fatalf("BenchmarkHeavyChanserv: frames != %d", floodSourceLimit*floodFrameLimit)
		}
	}
}

func getChanservAstranet() (Client, Server) {
	mpx := astranet.New().Server()
	if err := mpx.ListenAndServe("tcp4", ":5555"); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}
	srv := NewServer(mpx, ServerOnError(func(err error) {
		log.Println("[server WARN]", err)
	}))
	emptySrcFunc := getBenchSrcFunc(1, 1, 0)
	if err := srv.ListenAndServe("chanserv-astranet-empty", emptySrcFunc); err != nil {
		log.Fatalln("[server ERR]", err)
	}
	floodSrcFunc := getBenchSrcFunc(floodSourceLimit, floodFrameLimit, floodFrameSize)
	if err := srv.ListenAndServe("chanserv-astranet-flood", floodSrcFunc); err != nil {
		log.Fatalln("[server ERR]", err)
	}
	heavySrcFunc := getBenchSrcFunc(heavySourceLimit, heavyFrameLimit, heavyFrameSize)
	if err := srv.ListenAndServe("chanserv-astranet-heavy", heavySrcFunc); err != nil {
		log.Fatalln("[server ERR]", err)
	}
	mpx2 := astranet.New().Client()
	if err := mpx2.Join("tcp4", "localhost:5555"); err != nil {
		log.Fatalln("[astranet ERR]", err)
	}
	cli := NewClient(mpx2,
		ClientSourceBufferSize(1000),
		ClientFrameBufferSize(30000),
		ClientDialTimeout(5*time.Second),
		ClientOnError(func(err error) {
			log.Println("[client WARN]", err)
		}),
	)
	return cli, srv
}

func getBenchSrcFunc(srcLimit, frameLimit, frameSize int) func(req []byte) <-chan Source {
	return func(req []byte) <-chan Source {
		out := make(chan Source, srcLimit)
		go func() {
			for i := 0; i < srcLimit; i++ {
				out <- newBenchSrc(frameLimit, frameSize)
			}
			close(out)
		}()
		return out
	}
}

type benchSrc struct {
	frames int
	size   int
	buf    chan Frame
}

func newBenchSrc(frames, size int) Source {
	src := benchSrc{
		frames: frames,
		size:   size,
		buf:    make(chan Frame, size),
	}
	go func() {
		for i := 0; i < frames; i++ {
			var contents []byte
			if size > 0 {
				contents = make([]byte, size)
			}
			src.buf <- benchFrame{contents}
		}
		close(src.buf)
	}()
	return src
}

func (e benchSrc) Header() []byte    { return nil }
func (e benchSrc) Meta() MetaData    { return nil }
func (e benchSrc) Out() <-chan Frame { return e.buf }

type benchFrame struct {
	contents []byte
}

func (e benchFrame) Bytes() []byte {
	return e.contents
}

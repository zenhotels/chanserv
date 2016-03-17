package chanserv

import (
	"io"
	"log"
	"strings"
	"sync"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
)

func Connect(wg *sync.WaitGroup, addr string) error {
	skynet := skyproto_v1.SkyNetworkNew()
	conn, err := skynet.Dial(addr, "chanserv:1000")
	if err != nil {
		return err
	}
	defer conn.Close()

	req := []byte("hello")
	writeFrame(req, conn)

	wg.Add(1)
	defer wg.Done()

	frame, err := readFrame(conn)
	for err == nil {
		reply := string(frame)
		if !strings.HasPrefix(reply, "[HEAD]") {
			wg.Add(1)
			go discoverAddr(wg, skynet, addr, reply)
		}
		frame, err = readFrame(conn)
	}
	if err != io.EOF {
		return err
	}
	return nil
}

func discoverAddr(wg *sync.WaitGroup, skynet skyapi.Network, network, addr string) {
	defer wg.Done()

	conn, err := skynet.Dial(network, addr)
	if err != nil {
		log.Fatalln("[FATAL]: BOOM! Tried to discover an addr and found a mine:", err)
	}
	defer conn.Close()

	frame, err := readFrame(conn)
	for err == nil {
		log.Println("reply from", addr, string(frame))
		frame, err = readFrame(conn)
	}
	if err != io.EOF {
		log.Fatalln("[FATAL]: boom! discovery didn't end well")
	}
}

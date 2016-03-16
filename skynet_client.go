package chanserv

import (
	"io"
	"log"
	"strings"

	"hotcore.in/skynet/skyapi"
	"hotcore.in/skynet/skyapi/skyproto_v1"
)

func Connect(addr string) error {
	// HACK: dial quicker than listen
	// time.Sleep(time.Second)

	skynet := skyproto_v1.SkyNetworkNew(nil)
	log.Println("dialing to", addr, "skynet:12123123")
	conn, err := skynet.Dial(addr, "skynet:12123123")
	log.Println("dial error:", err)
	if err != nil {
		return err
	}
	defer conn.Close()

	req := []byte("hello")
	writeFrame(req, conn)

	frame, err := readFrame(conn)
	for err == nil {
		reply := string(frame)
		log.Println("master frame:", reply)
		if !strings.HasPrefix(reply, "[HEAD]") {
			discoverAddr(skynet, addr, reply)
		}
		frame, err = readFrame(conn)
	}
	if err != io.EOF {
		return err
	}
	log.Println("master closed")
	return nil
}

func discoverAddr(skynet skyapi.Network, network, addr string) {
	log.Println("dialing", addr, "in skynet over tcp", network)
	conn, err := skynet.Dial(network, addr)
	if err != nil {
		log.Fatalln("[FATAL]: BOOM! Tried to discover an addr and found a mine:", err)
	}
	defer conn.Close()

	log.Println("going for", addr)
	frame, err := readFrame(conn)
	for err == nil {
		log.Println("reply from", addr, string(frame))
		frame, err = readFrame(conn)
	}
	if err != io.EOF {
		log.Fatalln("[FATAL]: boom! discovery didn't end well")
	}
}

## Chanserv [![GoDoc](https://godoc.org/github.com/zenhotels/chanserv?status.svg)](https://godoc.org/github.com/zenhotels/chanserv)

```
$ go get github.com/astranet/chanserv
```

Package chanserv provides a simple message queue framework based upon nested Go-lang channels being served using [AstraNet](https://github.com/zenhotels/astranet).

### Wait, what?..

Explanatory post is here: http://tech.zenhotels.com/chanserv-and-astranet

### Demo output

```
$ go test
2016/03/17 21:11:36 chanserv_test.go:23: will connect in 3 sec...
2016/03/17 21:11:39 chanserv_test.go:44: [HEAD] source @0, for request: hello
2016/03/17 21:11:39 chanserv_test.go:44: [HEAD] source @1, for request: hello
2016/03/17 21:11:39 chanserv_test.go:44: [HEAD] source @2, for request: hello
2016/03/17 21:11:39 chanserv_test.go:44: [HEAD] source @3, for request: hello
2016/03/17 21:11:39 chanserv_test.go:44: [HEAD] source @4, for request: hello

2016/03/17 21:11:39 chanserv_test.go:48: [FRAME 0 from @5] wait for me!
2016/03/17 21:11:39 chanserv_test.go:48: [FRAME 0 from @1] wait for me!
2016/03/17 21:11:39 chanserv_test.go:48: [FRAME 0 from @4] wait for me!
2016/03/17 21:11:39 chanserv_test.go:48: [FRAME 0 from @3] wait for me!
2016/03/17 21:11:39 chanserv_test.go:48: [FRAME 0 from @2] wait for me!

2016/03/17 21:11:40 chanserv_test.go:48: [FRAME 1 from @1] ok I'm ready
2016/03/17 21:11:41 chanserv_test.go:48: [FRAME 1 from @2] ok I'm ready
2016/03/17 21:11:42 chanserv_test.go:48: [FRAME 1 from @3] ok I'm ready
2016/03/17 21:11:43 chanserv_test.go:48: [FRAME 1 from @4] ok I'm ready
2016/03/17 21:11:44 chanserv_test.go:48: [FRAME 1 from @5] ok I'm ready
PASS
```

## License

MIT

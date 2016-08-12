all:

test:
	go test -run=TestChanserv

bench:
	go test -run=none -bench=.

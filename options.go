package chanserv

import "time"

type ServerOption func(s *server)

func ServerOnError(fn func(err error)) ServerOption {
	return func(s *server) {
		s.onError = fn
	}
}

func ServerOnChanError(fn func(err error)) ServerOption {
	return func(s *server) {
		s.onChanError = fn
	}
}

func ServerMaxErrorMass(mass int) ServerOption {
	return func(s *server) {
		s.maxErrMass = mass
	}
}

func ServerOnMaxErrorMass(fn func(mass int, err error)) ServerOption {
	return func(s *server) {
		s.onMaxErrMass = fn
	}
}

func ServerServingTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.servingTimeout = d
	}
}

func ServerSourcingTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.sourcingTimeout = d
	}
}

func ServerChanAcceptTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.chanAcceptTimeout = d
	}
}

func ServerMasterReadTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.masterReadTimeout = d
	}
}

func ServerMasterWriteTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.masterWriteTimeout = d
	}
}

func ServerFrameReadTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.frameReadTimeout = d
	}
}

func ServerFrameWriteTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.frameWriteTimeout = d
	}
}

type ClientOption func(c *client)

func ClientOnError(fn func(err error)) ClientOption {
	return func(c *client) {
		c.onError = fn
	}
}

func ClientSourceBufferSize(size int) ClientOption {
	return func(c *client) {
		c.srcBufSize = size
	}
}

func ClientFrameBufferSize(size int) ClientOption {
	return func(c *client) {
		c.frameBufSize = size
	}
}

func ClientDialTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.dialTimeout = d
	}
}

func ClientMasterReadTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.masterReadTimeout = d
	}
}

func ClientMasterWriteTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.masterWriteTimeout = d
	}
}

func ClientFrameReadTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.frameReadTimeout = d
	}
}

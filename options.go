package chanserv

import "time"

// ServerOption applies a configuration option to the server.
type ServerOption func(s *server)

// ServerOnError specifies a function to call upon master chan serving error.
func ServerOnError(fn func(err error)) ServerOption {
	return func(s *server) {
		s.onError = fn
	}
}

// ServerOnChanError specifies a function to call upon frame chan serving error.
func ServerOnChanError(fn func(err error)) ServerOption {
	return func(s *server) {
		s.onChanError = fn
	}
}

// ServerMaxErrorMass specifies amount of sequential serving errors before considering
// doing some action, usually to sleep 30 seconds before a new retry.
func ServerMaxErrorMass(mass int) ServerOption {
	return func(s *server) {
		s.maxErrMass = mass
	}
}

// ServerOnMaxErrorMass specifies the action to perform when error mass is critical enough.
// By default it sleeps 30 seconds before a new retry.
func ServerOnMaxErrorMass(fn func(mass int, err error)) ServerOption {
	return func(s *server) {
		s.onMaxErrMass = fn
	}
}

// ServerServingTimeout specifies the overall serving timeout. This timeout sets a deadline
// for the master channel and all the descendant frame channels.
func ServerServingTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.servingTimeout = d
	}
}

// ServerSourcingTimeout specifies the timeout of source announcements
// in the master channel. The sourcing func must publish all the sources
// and close the sourcing channel before this timeout expires.
func ServerSourcingTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.sourcingTimeout = d
	}
}

// ServerChanAcceptTimeout specifies the timeout of frame channels
// waiting for being discovered by the client. Default value: 30s.
func ServerChanAcceptTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.chanAcceptTimeout = d
	}
}

// ServerMasterReadTimeout specifies the timeout for reading the
// client's request body after the master connection has been accepted.
func ServerMasterReadTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.masterReadTimeout = d
	}
}

// ServerMasterWriteTimeout specifies the timeout for writing the
// source announcement parts to the master connection (two parts).
func ServerMasterWriteTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.masterWriteTimeout = d
	}
}

// ServerFrameWriteTimeout specifies the timeout for writing the
// frame bytes to the descendant chan connection.
func ServerFrameWriteTimeout(d time.Duration) ServerOption {
	return func(s *server) {
		s.timeouts.frameWriteTimeout = d
	}
}

// ClientOption applies a configuration option to the client.
type ClientOption func(c *client)

// ClientOnError specifies a function to call upon an error.
func ClientOnError(fn func(err error)) ClientOption {
	return func(c *client) {
		c.onError = fn
	}
}

// ClientSourceBufferSize specifies the source announcements channel buffer size.
// Default value: 128.
func ClientSourceBufferSize(size int) ClientOption {
	return func(c *client) {
		c.srcBufSize = size
	}
}

// ClientFrameBufferSize specifies the frame channel buffer size.
// Default value: 1024.
func ClientFrameBufferSize(size int) ClientOption {
	return func(c *client) {
		c.frameBufSize = size
	}
}

// ClientDialTimeout specifies the timeout for dialing the remote host.
// Default value: 10s.
func ClientDialTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.dialTimeout = d
	}
}

// ClientMasterReadTimeout specifies the timeout for reading a
// source announcement bytes from the master connection.
func ClientMasterReadTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.masterReadTimeout = d
	}
}

// ClientMasterWriteTimeout specifies the timeout for writing the
// request body bytes after the connection has been accepted by the server.
func ClientMasterWriteTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.masterWriteTimeout = d
	}
}

// ClientFrameReadTimeout specifies the timeout for reading a frame
// data bytes from descendant chan connections that have benn discovered.
func ClientFrameReadTimeout(d time.Duration) ClientOption {
	return func(c *client) {
		c.timeouts.frameReadTimeout = d
	}
}

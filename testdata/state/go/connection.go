package connection

type State string

const (
	Closed     State = "closed"
	Open       State = "open"
	Unused     State = "unused"
	AlsoClosed State = "closed"
)

type Connection struct {
	state State
}

func (c *Connection) Open() {
	if c.state == Closed {
		c.state = Open
	}
}

func (c *Connection) Close() {
	if c.state == Open {
		c.state = Closed
	}
}

func (c *Connection) Set(next State) {
	c.state = next
}

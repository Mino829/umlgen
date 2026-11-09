package minimal

type Request struct {
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	CheckpointID string `json:"checkpointId"`
	Nested       map[string][]struct {
		Point struct{ X, Y int }
	}
	Handler interface {
		Handle(value struct{ Name string }) error
	}
	Empty struct{}
}

func (r *Request) Transform(input struct {
	Value string
}) struct {
	Result string
} {
	return struct{ Result string }{Result: input.Value}
}

func (r *Request) Accept(next string) {}

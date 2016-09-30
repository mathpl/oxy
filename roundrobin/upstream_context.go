package roundrobin

type UpstreamContext interface {
	GetServer() string
	SetServer(string)

	GetStatus() int
	SetStatus(int)
}

func NewUpstreamContext() UpstreamContext {
	return &upstreamContext{}
}

type upstreamContext struct {
	server string
	status int
}

func (uc *upstreamContext) GetServer() string {
	return uc.server
}

func (uc *upstreamContext) SetServer(s string) {
	uc.server = s
}

func (uc *upstreamContext) GetStatus() int {
	return uc.status
}

func (uc *upstreamContext) SetStatus(s int) {
	uc.status = s
}

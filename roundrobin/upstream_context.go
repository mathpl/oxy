package roundrobin

type UpstreamContext interface {
	Get(string) string
	Set(string, string)

	GetStatus() int
	SetStatus(int)
}

func NewUpstreamContext() UpstreamContext {
	return &upstreamContext{values: make(map[string]string, 5)}
}

type upstreamContext struct {
	status int
	values map[string]string
}

func (uc *upstreamContext) Get(k string) string {
	if uc.values != nil {
		return uc.values[k]
	} else {
		return ""
	}
}

func (uc *upstreamContext) Set(k string, v string) {
	if uc.values != nil {
		uc.values[k] = v
	}
}

func (uc *upstreamContext) GetStatus() int {
	return uc.status
}

func (uc *upstreamContext) SetStatus(s int) {
	uc.status = s
}

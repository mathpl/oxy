// package stickysession is a mixin for load balancers that implements layer 7 (http cookie) session affinity
package roundrobin

import (
	"net/http"
	"net/url"
)

type StickySession struct {
	cookiename string
}

func NewStickySession(c string) *StickySession {
	return &StickySession{c}
}

// GetBackend returns the backend URL stored in the sticky cookie, iff the backend is still in the valid list of servers.
func (s *StickySession) GetBackend(req *http.Request, servers []*server) (*server, bool, error) {
	cookie, err := req.Cookie(s.cookiename)
	switch err {
	case nil:
	case http.ErrNoCookie:
		return nil, false, nil
	default:
		return nil, false, err
	}

	s_url, err := url.Parse(cookie.Value)
	if err != nil {
		return nil, false, err
	}

	if srv := s.isBackendAlive(s_url, servers); srv != nil {
		return srv, true, nil
	} else {
		return nil, false, nil
	}
}

func (s *StickySession) StickBackend(backend *server, w *http.ResponseWriter) {
	c := &http.Cookie{Name: s.cookiename, Value: backend.url.String()}
	http.SetCookie(*w, c)
	return
}

func (s *StickySession) isBackendAlive(needle *url.URL, haystack []*server) *server {
	if len(haystack) == 0 {
		return nil
	}

	for _, s := range haystack {
		if sameURL(needle, s.url) {
			return s
		}
	}
	return nil
}

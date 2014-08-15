package etcdclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const (
	v2Prefix = "/v2/keys"
)

type Client struct {
	endpoints []string
	http.Client
}

var (
	ErrUnavailable = errors.New("client: no available etcd endpoints")
	ErrNoleader    = errors.New("client: no leader")
	ErrExists      = errors.New("client: key already exists")
)

func (c *Client) Create(key string, value string, ttl uint64) error {
	v := url.Values{}
	v.Add("value", value)
	v.Add("ttl", fmt.Sprintf("%d", ttl))
	r, err := http.NewRequest("PUT", path.Join(v2Prefix, key), ResetCloser(strings.NewReader(v.Encode())))
	if err != nil {
		panic(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.roundRobin(r)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return ErrExists
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return ErrNoleader
	}
	return nil
}

func (c *Client) Get(key string) (*Response, error) {
	r, err := http.NewRequest("GET", path.Join(v2Prefix, key), nil)
	if err != nil {
		panic(err)
	}
	httpresp, err := c.roundRobin(r)
	resp := &Response{}
	d := json.NewDecoder(httpresp.Body)
	if err = d.Decode(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) roundRobin(r *http.Request) (*http.Response, error) {
	for i := range c.endpoints {
		r.URL.Scheme = "http"
		r.URL.Host = c.endpoints[i]
		resp, err := c.do(r)
		if err == nil {
			return resp, err
		}
	}
	return nil, ErrUnavailable
}

func (c *Client) do(r *http.Request) (*http.Response, error) {
	for {
		resp, err := c.Client.Do(r)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTemporaryRedirect {
			u, err := resp.Location()
			if err != nil {
				panic(err)
			}
			r.URL = u
			continue
		}
		return resp, err
	}
}

type resetCloser struct{ *strings.Reader }

func (c resetCloser) Close() error {
	c.Reader.Seek(0, 0)
	return nil
}

func ResetCloser(r *strings.Reader) io.ReadCloser { return resetCloser{r} }

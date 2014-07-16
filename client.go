package thesrc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/google/go-querystring/query"
	"github.com/sourcegraph/thesrc/router"
)

// A Client communicates with thesrc's HTTP API.
type Client struct {
	Posts PostsService

	// BaseURL for HTTP requests to thesrc's API.
	BaseURL *url.URL

	//UserAgent used for HTTP requests to thesrc's API.
	UserAgent string

	httpClient *http.Client
}

const (
	libraryVersion = "0.0.1"
	userAgent      = "thesrc-client/" + libraryVersion
)

// NewClient creates a new HTTP API client for thesrc. If httpClient == nil,
// then http.DefaultClient is used.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	c := &Client{
		BaseURL:    &url.URL{Scheme: "http", Host: "thesrc.org", Path: "/api/"},
		UserAgent:  userAgent,
		httpClient: httpClient,
	}
	c.Posts = &postsService{c}
	return c
}

// apiRouter is used to generate URLs for thesrc's HTTP API.
var apiRouter = router.API()

// url generates the URL to the named thesrc API endpoint, using the
// specified route variables and query options.
func (c *Client) url(apiRouteName string, routeVars map[string]string, opt interface{}) (*url.URL, error) {
	route := apiRouter.Get(apiRouteName)
	if route == nil {
		return nil, fmt.Errorf("no API route named %q", apiRouteName)
	}

	routeVarsList := make([]string, 2*len(routeVars))
	i := 0
	for name, val := range routeVars {
		routeVarsList[i*2] = name
		routeVarsList[i*2+1] = val
		i++
	}
	url, err := route.URL(routeVarsList...)
	if err != nil {
		return nil, err
	}

	// make the route URL path relative to BaseURL by trimming the leading "/"
	url.Path = strings.TrimPrefix(url.Path, "/")

	if opt != nil {
		err = addOptions(url, opt)
		if err != nil {
			return nil, err
		}
	}

	return url, nil
}

// NewRequest creates an API request. A relative URL can be provided in urlStr,
// in which case it is resolved relative to the BaseURL of the Client. Relative
// URLs should always be specified without a preceding slash. If specified, the
// value pointed to by body is JSON encoded and included as the request body.
func (c *Client) NewRequest(method, urlStr string, body interface{}) (*http.Request, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	u := c.BaseURL.ResolveReference(rel)

	buf := new(bytes.Buffer)
	if body != nil {
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", c.UserAgent)
	return req, nil
}

// Do sends an API request and returns the API response. The API response is
// JSON-decoded and stored in the value pointed to by v, or returned as an error
// if an API error has occurred.
func (c *Client) Do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	err = CheckResponse(resp)
	if err != nil {
		// even though there was an error, we still return the response
		// in case the caller wants to inspect it further
		return resp, err
	}

	if v != nil {
		if bp, ok := v.(*[]byte); ok {
			*bp, err = ioutil.ReadAll(resp.Body)
		} else {
			err = json.NewDecoder(resp.Body).Decode(v)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error reading response from %s %s: %s", req.Method, req.URL.RequestURI(), err)
	}
	return resp, nil
}

// addOptions adds the parameters in opt as URL query parameters to u. opt
// must be a struct whose fields may contain "url" tags.
func addOptions(u *url.URL, opt interface{}) error {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}

	qs, err := query.Values(opt)
	if err != nil {
		return err
	}

	u.RawQuery = qs.Encode()
	return nil
}
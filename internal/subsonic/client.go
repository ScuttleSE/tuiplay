// Package subsonic is a client for the Subsonic / OpenSubsonic API.
//
// Navidrome implements this API. The client authenticates with a salted
// MD5 token. For each request the client makes a random salt. The client
// computes the token as md5(password + salt). The client sends the token
// as parameter t and the salt as parameter s. The client does not send
// the clear password.
//
// The client requests JSON with the parameter f=json.
package subsonic

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// apiVersion is the Subsonic REST protocol version that the client sends.
const apiVersion = "1.16.1"

// clientName identifies this application to the server.
const clientName = "tuiplay"

// Client talks to one Navidrome server.
type Client struct {
	baseURL    string
	username   string
	password   string
	maxBitRate int
	http       *http.Client

	// extensions holds the OpenSubsonic extensions the server supports.
	// The key is the extension name. The value is the supported version
	// list. A nil map means the client has not queried the server yet.
	extensions map[string][]int
}

// New makes a client for the server at baseURL.
func New(baseURL, username, password string, maxBitRate int) *Client {
	return &Client{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		maxBitRate: maxBitRate,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// makeSalt returns a random hex string for one request.
func makeSalt() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// token computes md5(password + salt) as a lower-case hex string.
func token(password, salt string) string {
	sum := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(sum[:])
}

// authParams returns the shared authentication query parameters.
func (c *Client) authParams() (url.Values, error) {
	salt, err := makeSalt()
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("u", c.username)
	v.Set("t", token(c.password, salt))
	v.Set("s", salt)
	v.Set("v", apiVersion)
	v.Set("c", clientName)
	v.Set("f", "json")
	return v, nil
}

// requestURL builds the full URL for an endpoint with extra parameters.
func (c *Client) requestURL(endpoint string, extra url.Values) (string, error) {
	v, err := c.authParams()
	if err != nil {
		return "", err
	}
	for key, vals := range extra {
		for _, val := range vals {
			v.Add(key, val)
		}
	}
	return fmt.Sprintf("%s/rest/%s?%s", c.baseURL, endpoint, v.Encode()), nil
}

// get calls a JSON endpoint and decodes the response into out.
func (c *Client) get(endpoint string, extra url.Values, out interface{}) error {
	u, err := c.requestURL(endpoint, extra)
	if err != nil {
		return err
	}
	resp, err := c.http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

// LoadExtensions queries the server for its OpenSubsonic extensions and
// caches the result on the client. The getOpenSubsonicExtensions endpoint
// is publicly accessible. A plain Subsonic server returns no extension
// list; the client then treats every extension as unsupported. A network
// or decode error is returned to the caller, who may treat it as soft.
func (c *Client) LoadExtensions() error {
	var r struct {
		Response struct {
			baseResponse
			OpenSubsonicExtensions []struct {
				Name     string `json:"name"`
				Versions []int  `json:"versions"`
			} `json:"openSubsonicExtensions"`
		} `json:"subsonic-response"`
	}
	if err := c.get("getOpenSubsonicExtensions", nil, &r); err != nil {
		return err
	}
	m := make(map[string][]int, len(r.Response.OpenSubsonicExtensions))
	for _, e := range r.Response.OpenSubsonicExtensions {
		m[e.Name] = e.Versions
	}
	c.extensions = m
	return nil
}

// HasExtension reports whether the server supports the named OpenSubsonic
// extension. It returns false until LoadExtensions has run.
func (c *Client) HasExtension(name string) bool {
	if c.extensions == nil {
		return false
	}
	_, ok := c.extensions[name]
	return ok
}

// StreamURL returns the URL that streams the media file with the given id.
func (c *Client) StreamURL(id string) (string, error) {
	extra := url.Values{}
	extra.Set("id", id)
	if c.maxBitRate > 0 {
		extra.Set("maxBitRate", fmt.Sprintf("%d", c.maxBitRate))
	}
	return c.requestURL("stream", extra)
}

// StreamURLFormat returns a stream URL that asks the server to transcode
// the file to the given format, for example "mp3". The server transcodes
// on the fly. An empty format lets the server choose. Use this for source
// files the client cannot decode itself, such as m4a (AAC or ALAC).
func (c *Client) StreamURLFormat(id, format string) (string, error) {
	extra := url.Values{}
	extra.Set("id", id)
	if format != "" {
		extra.Set("format", format)
	}
	if c.maxBitRate > 0 {
		extra.Set("maxBitRate", fmt.Sprintf("%d", c.maxBitRate))
	}
	return c.requestURL("stream", extra)
}

// CoverArtURL returns the URL of the cover art for the given id.
func (c *Client) CoverArtURL(id string) (string, error) {
	extra := url.Values{}
	extra.Set("id", id)
	return c.requestURL("getCoverArt", extra)
}

// CoverArt downloads the cover art image bytes for the given id. When size
// is positive, it asks the server to scale the image to that many pixels on
// its longest side. The getCoverArt endpoint returns raw image bytes, not
// JSON, so this cannot use the get helper. It returns the bytes and the
// response content type.
func (c *Client) CoverArt(id string, size int) ([]byte, string, error) {
	extra := url.Values{}
	extra.Set("id", id)
	if size > 0 {
		extra.Set("size", fmt.Sprintf("%d", size))
	}
	u, err := c.requestURL("getCoverArt", extra)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("getCoverArt: status %d", resp.StatusCode)
	}
	// A Subsonic error comes back as JSON with a subsonic-response wrapper.
	// A real image never starts with '{', so a JSON body signals an error.
	ct := resp.Header.Get("Content-Type")
	if len(data) > 0 && data[0] == '{' {
		return nil, "", fmt.Errorf("getCoverArt: server returned no image")
	}
	return data, ct, nil
}

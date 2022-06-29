//nolint
package traefik_plugin_rewrite_headers

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/posener/wstest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		desc          string
		rewrites      []Rewrite
		reqHeader     http.Header
		expRespHeader http.Header
	}{
		{
			desc: "should replace foo by bar in location header",
			rewrites: []Rewrite{
				{
					Header:      "Location",
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			reqHeader: map[string][]string{
				"Location": {"foo", "anotherfoo"},
			},
			expRespHeader: map[string][]string{
				"Location": {"bar", "anotherbar"},
			},
		},
		{
			desc: "should replace http by https in location header",
			rewrites: []Rewrite{
				{
					Header:      "Location",
					Regex:       "^http://(.+)$",
					Replacement: "https://$1",
				},
			},
			reqHeader: map[string][]string{
				"Location": {"http://test:1000"},
			},
			expRespHeader: map[string][]string{
				"Location": {"https://test:1000"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			config := &Config{
				Rewrites: test.rewrites,
			}

			next := func(rw http.ResponseWriter, req *http.Request) {
				for k, v := range test.reqHeader {
					for _, h := range v {
						rw.Header().Add(k, h)
					}
				}
				// handle websocket upgrade requests
				if strings.HasSuffix(req.URL.Path, "/ws") {
					upgrader := websocket.Upgrader{
						ReadBufferSize:  1024,
						WriteBufferSize: 1024,
					}
					conn, err := upgrader.Upgrade(rw, req, nil)
					if err != nil {
						fmt.Printf("Error in websocket upgrade: %v\n", err)
					}

					_, _, _ = conn.ReadMessage()
					fmt.Printf("Done upgrading\n")
					return
				}
				rw.WriteHeader(http.StatusOK)
			}

			rewriteBody, err := New(context.Background(), http.HandlerFunc(next), config, "rewriteHeader")
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			rewriteBody.ServeHTTP(recorder, req)

			// test websocket too
			resp := testWebsocket(t, rewriteBody)

			for k, expected := range test.expRespHeader {
				values := recorder.Header().Values(k)

				if !testEq(values, expected) {
					t.Errorf("Slices are not equal: expect: %+v, result: %+v", expected, values)
				}

				if resp.StatusCode != http.StatusSwitchingProtocols {
					t.Errorf("Websocket upgrade error")
				}
			}
		})
	}
}

func testWebsocket(t *testing.T, h http.Handler) *http.Response {
	d := wstest.NewDialer(h)
	c, resp, err := d.Dial("ws://host/ws", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = c.WriteJSON("test socket")
	if err != nil {
		t.Fatal(err)
	}

	// close the connection
	err = c.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func testEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

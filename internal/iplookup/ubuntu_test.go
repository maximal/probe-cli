package iplookup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/ooni/probe-cli/v3/internal/model"
	"github.com/ooni/probe-cli/v3/internal/netxlite"
)

// This test ensures that we correctly handle errors in Client.LookupWithUbuntu.
func TestClientLookupWithUbuntu(t *testing.T) {
	// testcase is a test case in this test
	type testcase struct {
		// name is the test case name
		name string

		// fx is the function to initialize TestingHTTPDo
		fx func(req *http.Request) ([]byte, error)

		// expectErr is the expected error
		expectErr error

		// expectAddr is the expected IP addr
		expectedAddr string
	}

	// errMocked an error returned to pretend that something failed.
	errMocked := errors.New("mocked error")

	// testcases contains all the test cases.
	testcases := []testcase{{
		name: "httpDo fails",
		fx: func(req *http.Request) ([]byte, error) {
			return nil, errMocked
		},
		expectErr:    errMocked,
		expectedAddr: "",
	}, {
		name: "the response is empty",
		fx: func(req *http.Request) ([]byte, error) {
			return nil, nil
		},
		expectErr:    io.EOF,
		expectedAddr: "",
	}, {
		name: "the response contains an invalid IP address",
		fx: func(req *http.Request) ([]byte, error) {
			response := []byte(`<Response>
			<Ip>1.ehlo.4.7</Ip>
			</Response>`)
			return response, nil
		},
		expectErr:    ErrInvalidIPAddress,
		expectedAddr: "",
	}, {
		name: "the response contains a valid IP address",
		fx: func(req *http.Request) ([]byte, error) {
			response := []byte(`<Response>
			<Ip>1.4.4.7</Ip>
			</Response>`)
			return response, nil
		},
		expectErr:    nil,
		expectedAddr: "1.4.4.7",
	}, {
		name: "we set a deadline for the request context",
		fx: func(req *http.Request) ([]byte, error) {
			ctx := req.Context()
			if _, ok := ctx.Deadline(); !ok {
				return nil, errors.New("missing deadline")
			}
			return []byte("<Response><Ip>1.4.4.7</Ip></Response>"), nil
		},
		expectErr:    nil,
		expectedAddr: "1.4.4.7",
	}}

	// run each test case
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// create a suitable client instance
			c := &Client{
				Logger:        model.DiscardLogger,
				Resolver:      netxlite.NewStdlibResolver(model.DiscardLogger),
				TestingHTTPDo: tc.fx,
			}

			// attempt to lookup
			addr, err := c.LookupIPAddr(context.Background(), MethodWebUbuntu, FamilyINET)

			// make sure the error is the expected one
			if !errors.Is(err, tc.expectErr) {
				t.Fatal("unexpected error", err)
			}

			// make sure the address is the expected one
			if addr != tc.expectedAddr {
				t.Fatal("expected ", tc.expectErr, "got", addr)
			}
		})
	}
}

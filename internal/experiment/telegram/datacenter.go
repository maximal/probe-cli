package telegram

//
// DatacenterTask
//

import (
	"context"

	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ooni/probe-cli/v3/internal/atomicx"
	"github.com/ooni/probe-cli/v3/internal/measurexlite"
	"github.com/ooni/probe-cli/v3/internal/model"
	"github.com/ooni/probe-cli/v3/internal/netxlite"
)

//
// Autogenerated section
//
// Suggestion: keep changes in this section minimal to facilitate
// generating the code again next time.
//
// You should insert your own code at the bottom.
//

// Measures a Telegram data center (DC).
//
// The zero value of this structure IS NOT valid and you MUST initialize
// all the fields marked as MANDATORY before using this structure.
//
// This task implements the http template.
type DatacenterTask struct {
	// Address is the MANDATORY address to connect to.
	Address string

	// IDGenerator is the MANDATORY atomic int64 to generate task IDs.
	IDGenerator *atomicx.Int64

	// Logger is the MANDATORY logger to use.
	Logger model.Logger

	// TestKeys is MANDATORY and contains the TestKeys.
	TestKeys *TestKeys

	// ZeroTime is the MANDATORY measurement's zero time.
	ZeroTime time.Time

	// WaitGroup is the MANDATORY wait group this task belongs to.
	WaitGroup *sync.WaitGroup

	// HostHeader is the OPTIONAL host header to use.
	HostHeader string

	// URLPath is the OPTIONAL URL path.
	URLPath string

	// URLRawQuery is the OPTIONAL URL raw query.
	URLRawQuery string
}

// Start starts this task in a background gorountine.
func (t *DatacenterTask) Start(ctx context.Context) {
	t.WaitGroup.Add(1)
	index := t.IDGenerator.Add(1)
	go t.run(ctx, index)
}

// run runs this task in the background.
func (t *DatacenterTask) run(parentCtx context.Context, index int64) {
	// synchronize with wait group
	defer t.WaitGroup.Done()

	// configure a timeout
	const defaultTimeout = 15 * time.Second // TODO: change this default
	opCtx, cancel := context.WithTimeout(parentCtx, defaultTimeout)
	defer cancel()

	// create trace
	trace := measurexlite.NewTrace(index, t.ZeroTime)

	// start the operation logger
	ol := measurexlite.NewOperationLogger(t.Logger, "Datacenter#%d: %s", index, t.Address)

	// perform the TCP connect
	tcpDialer := trace.NewDialerWithoutResolver(t.Logger)
	tcpConn, err := tcpDialer.DialContext(opCtx, "tcp", t.Address)
	_ = <-trace.TCPConnect // TODO: save
	if err != nil {
		ol.Stop(err)
		return
	}
	tcpConn = trace.WrapNetConn(tcpConn)
	defer func() {
		_ = trace.NetworkEvents() // TODO: save
		tcpConn.Close()
	}()

	// create HTTP transport
	httpTransport := netxlite.NewHTTPTransport(
		t.Logger,
		netxlite.NewSingleUseDialer(tcpConn),
		netxlite.NewNullTLSDialer(),
	)

	// create HTTP request
	httpReq, err := t.newHTTPRequest(opCtx)
	if err != nil {
		t.TestKeys.SetFundamentalFailure(err)
		ol.Stop(err)
		return
	}

	// perform HTTP round trip
	httpResp, httpRespBody, err := t.httpTransaction(opCtx, httpTransport, httpReq, trace)
	if err != nil {
		ol.Stop(err)
		return
	}

	// TODO: insert here additional code if needed
	_ = httpResp
	_ = httpRespBody

	// completed successfully
	ol.Stop(nil)
}

// urlHost computes the host to include into the URL
func (t *DatacenterTask) urlHost(scheme string) (string, error) {
	addr, port, err := net.SplitHostPort(t.Address)
	if err != nil {
		t.Logger.Warnf("BUG: net.SplitHostPort failed for %s: %s", t.Address, err.Error())
		return "", err
	}
	if port == "80" && scheme == "http" {
		return addr, nil
	}
	return t.Address, nil // there was no need to parse after all 😬
}

// newHTTPRequest creates a new HTTP request.
func (t *DatacenterTask) newHTTPRequest(ctx context.Context) (*http.Request, error) {
	const urlScheme = "http"
	urlHost, err := t.urlHost(urlScheme)
	if err != nil {
		return nil, err
	}
	httpURL := &url.URL{
		Scheme:   urlScheme,
		Host:     urlHost,
		Path:     t.URLPath,
		RawQuery: t.URLRawQuery,
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", httpURL.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Host", t.HostHeader)
	httpReq.Header.Set("Accept", model.HTTPHeaderAccept)
	httpReq.Header.Set("Accept-Language", model.HTTPHeaderAcceptLanguage)
	httpReq.Header.Set("User-Agent", model.HTTPHeaderUserAgent)
	return httpReq, nil
}

// httpTransaction runs the HTTP transaction and saves the results.
func (t *DatacenterTask) httpTransaction(ctx context.Context, txp model.HTTPTransport,
	req *http.Request, trace *measurexlite.Trace) (*http.Response, []byte, error) {
	const maxbody = 1 << 22 // TODO: you may want to change this default
	resp, err := txp.RoundTrip(req)
	_ = trace.NewArchivalHTTPRequestResult(txp, req, resp, maxbody, []byte{}, err) // TODO: save
	if err != nil {
		return resp, []byte{}, err
	}
	defer resp.Body.Close()
	reader := io.LimitReader(resp.Body, maxbody)
	body, err := netxlite.ReadAllContext(ctx, reader)
	_ = trace.NewArchivalHTTPRequestResult(txp, req, resp, maxbody, body, err) // TODO: save
	return resp, body, err
}

//
// User section
//
// We suggest adding your custom methods and functions here.
//

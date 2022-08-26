package telegram

//
// WebHTTP
//
// Generated by `boilerplate' using the http template.
//

import (
	"context"
	"errors"
	"io"
	"log"
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

// Measures Telegram Web using HTTP.
//
// The zero value of this structure IS NOT valid and you MUST initialize
// all the fields marked as MANDATORY before using this structure.
type WebHTTP struct {
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

	// CookieJar contains the OPTIONAL cookie jar, used for redirects.
	CookieJar http.CookieJar

	// HostHeader is the OPTIONAL host header to use.
	HostHeader string

	// URLPath is the OPTIONAL URL path.
	URLPath string

	// URLRawQuery is the OPTIONAL URL raw query.
	URLRawQuery string
}

// Start starts this task in a background goroutine.
func (t *WebHTTP) Start(ctx context.Context) {
	t.WaitGroup.Add(1)
	index := t.IDGenerator.Add(1)
	go func() {
		defer t.WaitGroup.Done() // synchronize with the parent
		t.Run(ctx, index)
	}()
}

// Run runs this task in the current goroutine.
func (t *WebHTTP) Run(parentCtx context.Context, index int64) {
	// create trace
	trace := measurexlite.NewTrace(index, t.ZeroTime)

	// start the operation logger
	ol := measurexlite.NewOperationLogger(t.Logger, "WebHTTP#%d: %s", index, t.Address)

	// perform the TCP connect
	const tcpTimeout = 10 * time.Second
	tcpCtx, tcpCancel := context.WithTimeout(parentCtx, tcpTimeout)
	defer tcpCancel()
	tcpDialer := trace.NewDialerWithoutResolver(t.Logger)
	tcpConn, err := tcpDialer.DialContext(tcpCtx, "tcp", t.Address)
	t.TestKeys.AppendTCPConnectResults(trace.TCPConnects()...)
	if err != nil {
		t.TestKeys.AppendWebFailure(err)
		ol.Stop(err)
		return
	}
	defer func() {
		t.TestKeys.AppendNetworkEvents(trace.NetworkEvents()...)
		tcpConn.Close()
	}()

	alpn := "" // no ALPN because we're not using TLS

	// create HTTP transport
	httpTransport := netxlite.NewHTTPTransport(
		t.Logger,
		netxlite.NewSingleUseDialer(tcpConn),
		netxlite.NewNullTLSDialer(),
	)

	// create HTTP request
	const httpTimeout = 10 * time.Second
	httpCtx, httpCancel := context.WithTimeout(parentCtx, httpTimeout)
	defer httpCancel()
	httpReq, err := t.newHTTPRequest(httpCtx)
	if err != nil {
		t.TestKeys.AppendWebFailure(err)
		t.TestKeys.SetFundamentalFailure(err)
		ol.Stop(err)
		return
	}

	// perform HTTP transaction
	httpResp, httpRespBody, err := t.httpTransaction(
		httpCtx,
		"tcp",
		t.Address,
		alpn,
		httpTransport,
		httpReq,
		trace,
	)
	if err != nil {
		t.TestKeys.AppendWebFailure(err)
		ol.Stop(err)
		return
	}

	// parse HTTP results
	if err := t.parseResults(httpResp, httpRespBody); err != nil {
		t.TestKeys.AppendWebFailure(err)
		ol.Stop(err)
		return
	}

	// completed successfully
	ol.Stop(nil)
}

// urlHost computes the host to include into the URL
func (t *WebHTTP) urlHost(scheme string) (string, error) {
	addr, port, err := net.SplitHostPort(t.Address)
	if err != nil {
		t.Logger.Warnf("BUG: net.SplitHostPort failed for %s: %s", t.Address, err.Error())
		return "", err
	}
	urlHost := t.HostHeader
	if urlHost == "" {
		urlHost = addr
	}
	if port == "80" && scheme == "http" {
		return urlHost, nil
	}
	urlHost = net.JoinHostPort(urlHost, port)
	return urlHost, nil
}

// newHTTPRequest creates a new HTTP request.
func (t *WebHTTP) newHTTPRequest(ctx context.Context) (*http.Request, error) {
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
	httpReq, err := http.NewRequestWithContext(ctx, "GET", httpURL.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Host", t.HostHeader)
	httpReq.Header.Set("Accept", model.HTTPHeaderAccept)
	httpReq.Header.Set("Accept-Language", model.HTTPHeaderAcceptLanguage)
	httpReq.Header.Set("User-Agent", model.HTTPHeaderUserAgent)
	httpReq.Host = t.HostHeader
	if t.CookieJar != nil {
		for _, cookie := range t.CookieJar.Cookies(httpURL) {
			httpReq.AddCookie(cookie)
		}
	}
	return httpReq, nil
}

// httpTransaction runs the HTTP transaction and saves the results.
func (t *WebHTTP) httpTransaction(ctx context.Context, network, address, alpn string,
	txp model.HTTPTransport, req *http.Request, trace *measurexlite.Trace) (*http.Response, []byte, error) {
	const maxbody = 1 << 19
	started := trace.TimeSince(trace.ZeroTime)
	resp, err := txp.RoundTrip(req)
	var body []byte
	if err == nil {
		defer resp.Body.Close()
		if cookies := resp.Cookies(); t.CookieJar != nil && len(cookies) > 0 {
			t.CookieJar.SetCookies(req.URL, cookies)
		}
		reader := io.LimitReader(resp.Body, maxbody)
		body, err = netxlite.ReadAllContext(ctx, reader)
	}
	finished := trace.TimeSince(trace.ZeroTime)
	ev := measurexlite.NewArchivalHTTPRequestResult(
		trace.Index,
		started,
		network,
		address,
		alpn,
		txp.Network(),
		req,
		resp,
		maxbody,
		body,
		err,
		finished,
	)
	t.TestKeys.AppendRequests(ev)
	return resp, body, err
}

// parseResults parses the results of this sub-measurement.
func (t *WebHTTP) parseResults(resp *http.Response, respBody []byte) error {
	if resp.StatusCode != 301 && resp.StatusCode != 308 {
		log.Printf("status code: %+v", resp.StatusCode)
		return errors.New("http_request_failed")
	}
	location, err := resp.Location()
	if err != nil {
		return errors.New("telegram_missing_redirect_error")
	}
	if location.Scheme != "https" || location.Host != webTelegramOrg {
		return errors.New("telegram_invalid_redirect_error")
	}
	return nil
}

package webconnectivity

//
// DNSResolvers
//
// This code was generated by `boilerplate' using
// the multi-resolver template.
//

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/ooni/probe-cli/v3/internal/atomicx"
	"github.com/ooni/probe-cli/v3/internal/measurexlite"
	"github.com/ooni/probe-cli/v3/internal/model"
	"github.com/ooni/probe-cli/v3/internal/netxlite"
)

// Resolves the URL's domain using several resolvers.
//
// The zero value of this structure IS NOT valid and you MUST initialize
// all the fields marked as MANDATORY before using this structure.
type DNSResolvers struct {
	// DNSCache is the MANDATORY DNS cache.
	DNSCache *DNSCache

	// Domain is the MANDATORY domain to resolve.
	Domain string

	// IDGenerator is the MANDATORY atomic int64 to generate task IDs.
	IDGenerator *atomicx.Int64

	// Logger is the MANDATORY logger to use.
	Logger model.Logger

	// TestKeys is MANDATORY and contains the TestKeys.
	TestKeys *TestKeys

	// URL is the MANDATORY URL we're measuring.
	URL *url.URL

	// ZeroTime is the MANDATORY zero time of the measurement.
	ZeroTime time.Time

	// WaitGroup is the MANDATORY wait group this task belongs to.
	WaitGroup *sync.WaitGroup

	// CookieJar contains the OPTIONAL cookie jar, used for redirects.
	CookieJar http.CookieJar

	// DNSOverHTTPSURL is the optional DoH URL to use. If this field is not
	// set, we use a default one (e.g., `https://mozilla.cloudflare-dns.com/dns-query`).
	DNSOverHTTPSURL string

	// Referer contains the OPTIONAL referer, used for redirects.
	Referer string

	// Session is the OPTIONAL session. If the session is set, we will use
	// it to start the task that issues the control request. This request must
	// only be sent during the first iteration. It would be pointless to
	// issue such a request for subsequent redirects.
	Session model.ExperimentSession

	// THAddr is the OPTIONAL test helper address.
	THAddr string

	// UDPAddress is the OPTIONAL address of the UDP resolver to use. If this
	// field is not set we use a default one (e.g., `8.8.8.8:53`).
	UDPAddress string
}

// Start starts this task in a background goroutine.
func (t *DNSResolvers) Start(ctx context.Context) {
	t.WaitGroup.Add(1)
	go func() {
		defer t.WaitGroup.Done() // synchronize with the parent
		t.Run(ctx)
	}()
}

// run performs a DNS lookup and returns the looked up addrs
func (t *DNSResolvers) run(parentCtx context.Context) []string {
	// create output channels for the lookup
	systemOut := make(chan []string)
	udpOut := make(chan []string)
	httpsOut := make(chan []string)

	// start asynchronous lookups
	go t.lookupHostSystem(parentCtx, systemOut)
	go t.lookupHostUDP(parentCtx, udpOut)
	go t.lookupHostDNSOverHTTPS(parentCtx, httpsOut)

	// collect resulting IP addresses (which may be nil/empty lists)
	systemAddrs := <-systemOut
	udpAddrs := <-udpOut
	httpsAddrs := <-httpsOut

	// merge the resolved IP addresses
	merged := map[string]bool{}
	for _, addr := range systemAddrs {
		merged[addr] = true
	}
	for _, addr := range udpAddrs {
		merged[addr] = true
	}
	for _, addr := range httpsAddrs {
		merged[addr] = true
	}

	// rearrange addresses to have IPv4 first
	sorted := []string{}
	for addr := range merged {
		if v6, err := netxlite.IsIPv6(addr); err == nil && !v6 {
			sorted = append(sorted, addr)
		}
	}
	for addr := range merged {
		if v6, err := netxlite.IsIPv6(addr); err == nil && v6 {
			sorted = append(sorted, addr)
		}
	}

	// TODO(bassosimone): remove bogons

	return sorted
}

// Run runs this task in the current goroutine.
func (t *DNSResolvers) Run(parentCtx context.Context) {
	var (
		addresses []string
		found     bool
	)

	// first attempt to use the dns cache
	addresses, found = t.DNSCache.Get(t.Domain)

	if !found {
		// fall back to performing a real dns lookup
		addresses = t.run(parentCtx)

		// insert the addresses we just looked us into the cache
		t.DNSCache.Set(t.Domain, addresses)
	}

	log.Infof("using: %+v", addresses)

	// fan out a number of child async tasks to use the IP addrs
	t.startCleartextFlows(parentCtx, addresses)
	t.startSecureFlows(parentCtx, addresses)
	t.maybeStartControlFlow(parentCtx, addresses)
}

// lookupHostSystem performs a DNS lookup using the system resolver.
func (t *DNSResolvers) lookupHostSystem(parentCtx context.Context, out chan<- []string) {
	// create context with attached a timeout
	const timeout = 4 * time.Second
	lookupCtx, lookpCancel := context.WithTimeout(parentCtx, timeout)
	defer lookpCancel()

	// create trace's index
	index := t.IDGenerator.Add(1)

	// create trace
	trace := measurexlite.NewTrace(index, t.ZeroTime)

	// start the operation logger
	ol := measurexlite.NewOperationLogger(
		t.Logger, "[#%d] lookup %s using system", index, t.Domain,
	)

	// runs the lookup
	reso := trace.NewStdlibResolver(t.Logger)
	addrs, err := reso.LookupHost(lookupCtx, t.Domain)
	t.TestKeys.AppendQueries(trace.DNSLookupsFromRoundTrip()...)
	ol.Stop(err)
	out <- addrs // must send something -even nil- to the parent
}

// lookupHostUDP performs a DNS lookup using an UDP resolver.
func (t *DNSResolvers) lookupHostUDP(parentCtx context.Context, out chan<- []string) {
	// create context with attached a timeout
	const timeout = 4 * time.Second
	lookupCtx, lookpCancel := context.WithTimeout(parentCtx, timeout)
	defer lookpCancel()

	// create trace's index
	index := t.IDGenerator.Add(1)

	// create trace
	trace := measurexlite.NewTrace(index, t.ZeroTime)

	// start the operation logger
	udpAddress := t.udpAddress()
	ol := measurexlite.NewOperationLogger(
		t.Logger, "[#%d] lookup %s using %s", index, t.Domain, udpAddress,
	)

	// runs the lookup
	dialer := netxlite.NewDialerWithoutResolver(t.Logger)
	reso := trace.NewParallelUDPResolver(t.Logger, dialer, udpAddress)
	addrs, err := reso.LookupHost(lookupCtx, t.Domain)

	// saves the results making sure we split Do53 queries from other queries
	do53, other := t.do53SplitQueries(trace.DNSLookupsFromRoundTrip())
	t.TestKeys.AppendQueries(do53...)
	t.TestKeys.WithTestKeysDo53(func(tkd *TestKeysDo53) {
		tkd.Queries = append(tkd.Queries, other...)
		tkd.NetworkEvents = append(tkd.NetworkEvents, trace.NetworkEvents()...)
	})

	ol.Stop(err)
	out <- addrs // must send something -even nil- to the parent
}

// Divides queries generated by Do53 in Do53-proper queries and other queries.
func (t *DNSResolvers) do53SplitQueries(
	input []*model.ArchivalDNSLookupResult) (do53, other []*model.ArchivalDNSLookupResult) {
	for _, query := range input {
		switch query.Engine {
		case "udp", "tcp":
			do53 = append(do53, query)
		default:
			other = append(other, query)
		}
	}
	return
}

// Returns the UDP resolver we should be using by default.
func (t *DNSResolvers) udpAddress() string {
	if t.UDPAddress != "" {
		return t.UDPAddress
	}
	return "8.8.4.4:53"
}

// lookupHostDNSOverHTTPS performs a DNS lookup using a DoH resolver.
func (t *DNSResolvers) lookupHostDNSOverHTTPS(parentCtx context.Context, out chan<- []string) {
	// create context with attached a timeout
	const timeout = 4 * time.Second
	lookupCtx, lookpCancel := context.WithTimeout(parentCtx, timeout)
	defer lookpCancel()

	// create trace's index
	index := t.IDGenerator.Add(1)

	// create trace
	trace := measurexlite.NewTrace(index, t.ZeroTime)

	// start the operation logger
	URL := t.dnsOverHTTPSURL()
	ol := measurexlite.NewOperationLogger(
		t.Logger, "[#%d] lookup %s using %s", index, t.Domain, URL,
	)

	// runs the lookup
	reso := trace.NewParallelDNSOverHTTPSResolver(t.Logger, URL)
	addrs, err := reso.LookupHost(lookupCtx, t.Domain)
	reso.CloseIdleConnections()

	// save results making sure we properly split DoH queries from other queries
	doh, other := t.dohSplitQueries(trace.DNSLookupsFromRoundTrip())
	t.TestKeys.Queries = append(t.TestKeys.Queries, doh...)
	t.TestKeys.WithTestKeysDoH(func(tkdh *TestKeysDoH) {
		tkdh.Queries = append(tkdh.Queries, other...)
		tkdh.NetworkEvents = append(tkdh.NetworkEvents, trace.NetworkEvents()...)
		tkdh.TCPConnect = append(tkdh.TCPConnect, trace.TCPConnects()...)
		tkdh.TLSHandshakes = append(tkdh.TLSHandshakes, trace.TLSHandshakes()...)
	})

	ol.Stop(err)
	out <- addrs // must send something -even nil- to the parent
}

// Divides queries generated by DoH in DoH-proper queries and other queries.
func (t *DNSResolvers) dohSplitQueries(
	input []*model.ArchivalDNSLookupResult) (doh, other []*model.ArchivalDNSLookupResult) {
	for _, query := range input {
		switch query.Engine {
		case "doh":
			doh = append(doh, query)
		default:
			other = append(other, query)
		}
	}
	return
}

// Returns the DOH resolver URL we should be using by default.
func (t *DNSResolvers) dnsOverHTTPSURL() string {
	if t.DNSOverHTTPSURL != "" {
		return t.DNSOverHTTPSURL
	}
	return "https://mozilla.cloudflare-dns.com/dns-query"
}

// startCleartextFlows starts a TCP measurement flow for each IP addr.
func (t *DNSResolvers) startCleartextFlows(ctx context.Context, addresses []string) {
	if t.URL.Scheme != "http" {
		// Do not bother with measuring HTTP when the user
		// has asked us to measure an HTTPS URL.
		return
	}
	sema := make(chan any, 1)
	sema <- true // allow a single flow to fetch the HTTP body
	port := "80"
	if urlPort := t.URL.Port(); urlPort != "" {
		port = urlPort
	}
	for _, addr := range addresses {
		task := &CleartextFlow{
			Address:         net.JoinHostPort(addr, port),
			DNSCache:        t.DNSCache,
			IDGenerator:     t.IDGenerator,
			Logger:          t.Logger,
			Sema:            sema,
			TestKeys:        t.TestKeys,
			ZeroTime:        t.ZeroTime,
			WaitGroup:       t.WaitGroup,
			CookieJar:       t.CookieJar,
			DNSOverHTTPSURL: t.DNSOverHTTPSURL,
			FollowRedirects: t.URL.Scheme == "http",
			HostHeader:      t.URL.Host,
			Referer:         t.Referer,
			UDPAddress:      t.UDPAddress,
			URLPath:         t.URL.Path,
			URLRawQuery:     t.URL.RawQuery,
		}
		task.Start(ctx)
	}
}

// startSecureFlows starts a TCP+TLS measurement flow for each IP addr.
func (t *DNSResolvers) startSecureFlows(ctx context.Context, addresses []string) {
	sema := make(chan any, 1)
	if t.URL.Scheme == "https" {
		// Allows just a single worker to fetch the response body but do that
		// only if the test-lists URL uses "https" as the scheme. Otherwise, just
		// validate IPs by performing a TLS handshake.
		sema <- true
	}
	port := "443"
	if urlPort := t.URL.Port(); urlPort != "" {
		if t.URL.Scheme != "https" {
			// If the URL is like http://example.com:8080/, we don't know
			// which would be the correct port where to use HTTPS.
			return
		}
		port = urlPort
	}
	for _, addr := range addresses {
		task := &SecureFlow{
			Address:         net.JoinHostPort(addr, port),
			DNSCache:        t.DNSCache,
			IDGenerator:     t.IDGenerator,
			Logger:          t.Logger,
			Sema:            sema,
			TestKeys:        t.TestKeys,
			ZeroTime:        t.ZeroTime,
			WaitGroup:       t.WaitGroup,
			ALPN:            []string{"h2", "http/1.1"},
			CookieJar:       t.CookieJar,
			DNSOverHTTPSURL: t.DNSOverHTTPSURL,
			FollowRedirects: t.URL.Scheme == "https",
			SNI:             t.URL.Hostname(),
			HostHeader:      t.URL.Host,
			Referer:         t.Referer,
			UDPAddress:      t.UDPAddress,
			URLPath:         t.URL.Path,
			URLRawQuery:     t.URL.RawQuery,
		}
		task.Start(ctx)
	}
}

// maybeStartControlFlow starts the control flow, when .Session is set.
func (t *DNSResolvers) maybeStartControlFlow(ctx context.Context, addresses []string) {
	if t.Session != nil && t.THAddr != "" {
		ctrl := &Control{
			Addresses: addresses,
			Logger:    t.Logger,
			TestKeys:  t.TestKeys,
			Session:   t.Session,
			THAddr:    t.THAddr,
			URL:       t.URL,
			WaitGroup: t.WaitGroup,
		}
		ctrl.Start(ctx)
	}
}

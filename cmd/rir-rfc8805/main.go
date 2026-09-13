// Copyright (C) 2026 Fastah Inc.

package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kenshaw/diskcache"
	"github.com/openrdap/rdap"
	"github.com/phemmer/go-iptrie"
	"go4.org/netipx"
)

var rirs = []string{"ripe", "afrinic", "apnic", "arin", "lacnic"}

type resultRow struct {
	Row           int    `json:"row"`
	Prefix        string `json:"prefix"`
	QueryIP       string `json:"queryIp"`
	RIR           string `json:"rir"`
	BootstrapRIR  string `json:"bootstrapRir,omitempty"`
	FinalRIR      string `json:"finalRir,omitempty"`
	FinalRDAPHost string `json:"finalRdapHost,omitempty"`
	NetworkName   string `json:"networkName,omitempty"`
	Error         string `json:"error,omitempty"`
}

type csvJob struct {
	RowNumber int
	Prefix    netip.Prefix
}

type rowResult struct {
	Job csvJob
	Row resultRow
	Err error
}

type httpCacheTransport struct {
	cache *diskcache.Cache

	mu       sync.Mutex
	diskHits int
	misses   int
}

type cacheStats struct {
	Requests   int
	MemoryHits int
	DiskHits   int
	Misses     int
	HitRate    float64
}

type semanticCache struct {
	mu      sync.Mutex
	trie    *iptrie.Trie
	hits    int
	misses  int
	inserts int
}

type semanticCacheStats struct {
	Lookups int
	Hits    int
	Misses  int
	Inserts int
	HitRate float64
}

type semanticCacheEntry struct {
	Network        *rdap.IPNetwork
	Classification rirClassification
}

func main() {
	defaultCacheDir, err := os.UserCacheDir()
	if err != nil {
		defaultCacheDir = os.TempDir()
	}
	defaultCacheDir = filepath.Join(defaultCacheDir, "rir-rfc8805", "rdap-http")

	inputPath := flag.String("input", "~/tmp/validated-all.csv", "RFC 8805 CSV input path")
	cacheDir := flag.String("cache-dir", defaultCacheDir, "directory for persistent RDAP HTTP cache")
	limit := flag.Int("limit", 0, "maximum number of CSV rows to process; 0 means all rows")
	maxConcurrentRDAP := flag.Int("max-concurrent-rdap", 5, "maximum concurrent RDAP lookups")
	rdapTimeout := flag.Duration("rdap-timeout", 30*time.Second, "maximum duration for one RDAP lookup")
	progressEvery := flag.Int("progress-every", 1000, "write progress to stderr every N rows; 0 disables progress")
	semanticCacheEnabled := flag.Bool("semantic-cache", false, "reuse RDAP network results for later IPs contained by the returned network range")
	flag.Parse()

	if err := run(*inputPath, *cacheDir, *limit, *maxConcurrentRDAP, *rdapTimeout, *progressEvery, *semanticCacheEnabled, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(inputPath string, cacheDir string, limit int, maxConcurrentRDAP int, rdapTimeout time.Duration, progressEvery int, semanticCacheEnabled bool, output io.Writer) error {
	inputPath = expandHome(inputPath)
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input CSV: %w", err)
	}
	defer input.Close()

	if maxConcurrentRDAP < 1 {
		maxConcurrentRDAP = 1
	}

	transport, err := newHTTPCacheTransport(newRDAPTransport(maxConcurrentRDAP), cacheDir)
	if err != nil {
		return err
	}
	defer func() {
		stats := transport.stats()
		fmt.Fprintf(os.Stderr, "rdap cache: requests=%d memoryHits=%d diskHits=%d misses=%d hitRate=%.1f%%\n", stats.Requests, stats.MemoryHits, stats.DiskHits, stats.Misses, stats.HitRate)
	}()

	httpClient := &http.Client{Transport: transport}
	var semanticCache *semanticCache
	if semanticCacheEnabled {
		semanticCache = newSemanticCache()
		defer func() {
			stats := semanticCache.stats()
			fmt.Fprintf(os.Stderr, "semantic cache: lookups=%d hits=%d misses=%d insertedPrefixes=%d hitRate=%.1f%%\n", stats.Lookups, stats.Hits, stats.Misses, stats.Inserts, stats.HitRate)
		}()
	} else {
		defer func() {
			fmt.Fprintln(os.Stderr, "semantic cache: disabled")
		}()
	}

	csvReader := csv.NewReader(input)
	csvReader.FieldsPerRecord = -1

	encoder := json.NewEncoder(output)
	rowNumber := 1
	jobs := make(chan csvJob, maxConcurrentRDAP*2)
	results := make(chan rowResult, maxConcurrentRDAP*2)
	producerErr := make(chan error, 1)
	var workers sync.WaitGroup
	for worker := 0; worker < maxConcurrentRDAP; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				row, err := processPrefix(job.RowNumber, job.Prefix, httpClient, semanticCache, rdapTimeout)
				results <- rowResult{Job: job, Row: row, Err: err}
			}
		}()
	}
	go func() {
		producerErr <- produceJobs(csvReader, &rowNumber, limit, jobs)
		close(jobs)
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	processed := 0
	rowErrors := 0
	defer func() {
		fmt.Fprintf(os.Stderr, "row errors: %d\n", rowErrors)
	}()
	for result := range results {
		if result.Err != nil {
			rowErrors++
			result.Row = resultRow{
				Row:     result.Job.RowNumber,
				Prefix:  result.Job.Prefix.String(),
				QueryIP: result.Job.Prefix.Addr().String(),
				Error:   result.Err.Error(),
			}
		}
		if err := encoder.Encode(result.Row); err != nil {
			return fmt.Errorf("write result for row %d: %w", result.Row.Row, err)
		}

		processed++
		if progressEvery > 0 && processed%progressEvery == 0 {
			fmt.Fprintf(os.Stderr, "processed %d rows\n", processed)
		}
	}
	if err := <-producerErr; err != nil {
		return err
	}
	return nil
}

func produceJobs(csvReader *csv.Reader, rowNumber *int, limit int, jobs chan<- csvJob) error {
	queued := 0
	for limit <= 0 || queued < limit {
		currentRow := *rowNumber
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read CSV row %d: %w", currentRow, err)
		}
		*rowNumber++
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}

		prefix, err := netip.ParsePrefix(strings.TrimSpace(record[0]))
		if err != nil {
			return fmt.Errorf("parse prefix at row %d (%q): %w", currentRow, record[0], err)
		}
		jobs <- csvJob{RowNumber: currentRow, Prefix: prefix.Masked()}
		queued++
	}

	return nil
}

func processPrefix(rowNumber int, prefix netip.Prefix, httpClient *http.Client, semanticCache *semanticCache, rdapTimeout time.Duration) (resultRow, error) {
	var network *rdap.IPNetwork
	var classification rirClassification
	if semanticCache != nil {
		if cached, ok := semanticCache.lookup(prefix.Addr()); ok {
			network = cached.Network
			classification = cached.Classification
		}
	}

	if network == nil {
		client := &rdap.Client{
			HTTP:      httpClient,
			UserAgent: "fastah-rir-rfc8805-experiment/0.1",
		}
		request := rdap.NewRequest(rdap.IPRequest, prefix.Addr().String())
		request.Timeout = rdapTimeout
		response, err := client.Do(request)
		if err != nil {
			return resultRow{}, fmt.Errorf("query RDAP for row %d prefix %s: %w", rowNumber, prefix, err)
		}

		var ok bool
		network, ok = response.Object.(*rdap.IPNetwork)
		if !ok {
			return resultRow{}, fmt.Errorf("query RDAP for row %d prefix %s: expected IP network response, got %T", rowNumber, prefix, response.Object)
		}
		classification = classifyRIR(response, network)
		if semanticCache != nil {
			semanticCache.insert(network, classification)
		}
	}

	return resultRow{
		Row:           rowNumber,
		Prefix:        prefix.String(),
		QueryIP:       prefix.Addr().String(),
		RIR:           classification.RIR,
		BootstrapRIR:  classification.BootstrapRIR,
		FinalRIR:      classification.FinalRIR,
		FinalRDAPHost: classification.FinalRDAPHost,
		NetworkName:   network.Name,
	}, nil
}

func newRDAPTransport(maxConcurrentRDAP int) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = maxConcurrentRDAP * 4
	transport.MaxIdleConnsPerHost = maxConcurrentRDAP
	transport.MaxConnsPerHost = maxConcurrentRDAP
	return transport
}

func newHTTPCacheTransport(base http.RoundTripper, dir string) (*httpCacheTransport, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	cache, err := diskcache.New(
		diskcache.WithRoot(dir),
		diskcache.WithTransport(base),
		diskcache.WithMethod(http.MethodGet),
		diskcache.WithTTL(365*24*time.Hour),
		diskcache.WithGzipCompression(),
	)
	if err != nil {
		return nil, fmt.Errorf("create RDAP HTTP disk cache: %w", err)
	}
	return &httpCacheTransport{
		cache: cache,
	}, nil
}

func (t *httpCacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cached, cacheErr := t.cache.Cached(req)
	if cacheErr == nil && cached {
		t.recordDiskHit()
	} else {
		t.recordMiss()
	}

	resp, err := t.cache.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if req.Method == http.MethodGet && (resp.StatusCode < 200 || resp.StatusCode >= 400) {
		_ = t.cache.Evict(req)
	}
	return resp, nil
}

func (t *httpCacheTransport) recordDiskHit() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.diskHits++
}

func (t *httpCacheTransport) recordMiss() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.misses++
}

func (t *httpCacheTransport) stats() cacheStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	requests := t.diskHits + t.misses
	stats := cacheStats{
		Requests: requests,
		DiskHits: t.diskHits,
		Misses:   t.misses,
	}
	if requests > 0 {
		stats.HitRate = float64(t.diskHits) / float64(requests) * 100
	}
	return stats
}

func newSemanticCache() *semanticCache {
	return &semanticCache{trie: iptrie.NewTrie()}
}

func (cache *semanticCache) lookup(addr netip.Addr) (*semanticCacheEntry, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	value := cache.trie.Find(addr)
	entry, ok := value.(*semanticCacheEntry)
	if !ok || entry == nil {
		cache.misses++
		return nil, false
	}

	cache.hits++
	return entry, true
}

func (cache *semanticCache) insert(network *rdap.IPNetwork, classification rirClassification) {
	prefixes := prefixesForRDAPNetwork(network)
	if len(prefixes) == 0 {
		return
	}

	entry := &semanticCacheEntry{
		Network:        network,
		Classification: classification,
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, prefix := range prefixes {
		cache.trie.Insert(prefix, entry)
		cache.inserts++
	}
}

func (cache *semanticCache) stats() semanticCacheStats {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	lookups := cache.hits + cache.misses
	stats := semanticCacheStats{
		Lookups: lookups,
		Hits:    cache.hits,
		Misses:  cache.misses,
		Inserts: cache.inserts,
	}
	if lookups > 0 {
		stats.HitRate = float64(cache.hits) / float64(lookups) * 100
	}
	return stats
}

func prefixesForRDAPNetwork(network *rdap.IPNetwork) []netip.Prefix {
	if network == nil || network.StartAddress == "" || network.EndAddress == "" {
		return nil
	}

	startAddress, err := netip.ParseAddr(network.StartAddress)
	if err != nil {
		return nil
	}
	endAddress, err := netip.ParseAddr(network.EndAddress)
	if err != nil {
		return nil
	}

	return netipx.IPRangeFrom(startAddress, endAddress).Prefixes()
}

type rirClassification struct {
	RIR           string
	BootstrapRIR  string
	FinalRIR      string
	FinalRDAPHost string
}

func classifyRIR(response *rdap.Response, network *rdap.IPNetwork) rirClassification {
	classification := rirClassification{}
	if response != nil {
		if response.BootstrapAnswer != nil && len(response.BootstrapAnswer.URLs) > 0 {
			classification.BootstrapRIR = rirFromURLString(response.BootstrapAnswer.URLs[0].String())
		}

		finalURL := finalRDAPURL(response.HTTP)
		classification.FinalRIR = rirFromURLString(finalURL)
		classification.FinalRDAPHost = hostFromURLString(finalURL)
	}

	classification.RIR = firstNonEmpty(classification.FinalRIR, classification.BootstrapRIR, inferRIRFromNetwork(network))
	return classification
}

func finalRDAPURL(responses []*rdap.HTTPResponse) string {
	for index := len(responses) - 1; index >= 0; index-- {
		response := responses[index]
		if response == nil || response.URL == "" {
			continue
		}
		if response.Response == nil || response.Response.StatusCode < 400 {
			return response.URL
		}
	}
	return ""
}

func rirFromURLString(rawURL string) string {
	host := hostFromURLString(rawURL)
	if host == "" {
		return ""
	}

	switch {
	case strings.Contains(host, "ripe.net"):
		return "ripe"
	case strings.Contains(host, "afrinic.net"):
		return "afrinic"
	case strings.Contains(host, "apnic.net"):
		return "apnic"
	case strings.Contains(host, "arin.net"):
		return "arin"
	case strings.Contains(host, "lacnic.net"):
		return "lacnic"
	default:
		return ""
	}
}

func hostFromURLString(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Hostname() == "" {
		return ""
	}
	return strings.ToLower(parsedURL.Hostname())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func inferRIRFromNetwork(network *rdap.IPNetwork) string {
	if network == nil {
		return ""
	}

	candidates := []string{network.Port43}
	for _, link := range network.Links {
		candidates = append(candidates, link.Value, link.Href, link.Title)
	}
	for _, notice := range network.Notices {
		candidates = append(candidates, notice.Title, notice.Type)
		candidates = append(candidates, notice.Description...)
		for _, link := range notice.Links {
			candidates = append(candidates, link.Value, link.Href, link.Title)
		}
	}
	for _, remark := range network.Remarks {
		candidates = append(candidates, remark.Title, remark.Type)
		candidates = append(candidates, remark.Description...)
		for _, link := range remark.Links {
			candidates = append(candidates, link.Value, link.Href, link.Title)
		}
	}

	text := strings.ToLower(strings.Join(candidates, " "))
	for _, rir := range rirs {
		if strings.Contains(text, rir) {
			return rir
		}
	}
	return ""
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const outfittingSource = "https://www.edsm.net"

type outfitStation struct {
	Key, System, Name, Type string
	MarketID                uint64
	Distance                *float64
	HaveOutfitting          bool
	Updated                 time.Time
}

type outfitModule struct {
	ID, Name, Category, Rating, Ship string
	Class                            *int
}

type outfitStationsResult struct {
	System    string
	Stations  []outfitStation
	Retrieved time.Time
	Cached    bool
}
type outfitModulesResult struct {
	Modules   []outfitModule
	Retrieved time.Time
	Cached    bool
}

type outfitCacheEntry struct {
	data    []byte
	fetched time.Time
}

// One on-demand client per view/session. Requests are serial, paced, bounded and
// cancellable. Keep-alive is disabled so there are no idle sockets after a query.
type outfittingClient struct {
	base       string
	http       *http.Client
	gate       chan struct{}
	mu         sync.Mutex
	cache      map[string]outfitCacheEntry
	cacheBytes int
	last       time.Time
	gap        time.Duration
	now        func() time.Time
}

func newOutfittingClient() *outfittingClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.MaxConnsPerHost = 1
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 7 * time.Second
	c := &outfittingClient{base: outfittingSource, http: &http.Client{Transport: transport, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, gate: make(chan struct{}, 1),
		cache: make(map[string]outfitCacheEntry), gap: time.Second, now: time.Now}
	c.gate <- struct{}{}
	return c
}

func outfitSystemName(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || len([]rune(input)) > 128 {
		return "", errors.New("enter a system name (up to 128 characters)")
	}
	for _, r := range input {
		if unicode.IsControl(r) {
			return "", errors.New("system name contains control characters")
		}
	}
	return input, nil
}

func (c *outfittingClient) get(parent context.Context, path string, values url.Values, refresh bool) ([]byte, time.Time, bool, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, false, err
	}
	key := path + "?" + values.Encode()
	readCache := func() ([]byte, time.Time, bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok := c.cache[key]
		age := c.now().Sub(entry.fetched)
		if ok && !refresh && age >= 0 && age < 10*time.Minute {
			return entry.data, entry.fetched, true
		}
		return nil, time.Time{}, false
	}
	if data, stamp, ok := readCache(); ok {
		return data, stamp, true, nil
	}
	select {
	case <-ctx.Done():
		return nil, time.Time{}, false, ctx.Err()
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	if data, stamp, ok := readCache(); ok {
		return data, stamp, true, nil
	}
	if wait := c.gap - c.now().Sub(c.last); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, time.Time{}, false, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, false, err
	}
	c.last = c.now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+key, nil)
	if err != nil {
		return nil, time.Time{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Co-pit/0.1 (Elite Dangerous station outfitting viewer)")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("EDSM request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusTooManyRequests {
			return nil, time.Time{}, false, errors.New("EDSM is rate-limiting requests; wait before trying again")
		}
		return nil, time.Time{}, false, fmt.Errorf("EDSM returned HTTP %d; try again later or use browser search", response.StatusCode)
	}
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("EDSM response interrupted: %w", err)
	}
	if len(data) > limit {
		return nil, time.Time{}, false, errors.New("EDSM response exceeds 2 MiB")
	}
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, false, err
	}
	stamp := c.now()
	return data, stamp, false, nil
}

// Only validated JSON enters the cache. Its size and number of entries are bounded.
func (c *outfittingClient) remember(path string, values url.Values, data []byte, stamp time.Time) {
	key := path + "?" + values.Encode()
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.cache[key]; ok {
		c.cacheBytes -= len(old.data)
		delete(c.cache, key)
	}
	for len(c.cache) >= 16 || c.cacheBytes+len(data) > 8<<20 {
		oldKey := ""
		var oldest time.Time
		for k, entry := range c.cache {
			if oldKey == "" || entry.fetched.Before(oldest) {
				oldKey, oldest = k, entry.fetched
			}
		}
		if oldKey == "" {
			break
		}
		c.cacheBytes -= len(c.cache[oldKey].data)
		delete(c.cache, oldKey)
	}
	c.cache[key] = outfitCacheEntry{data: data, fetched: stamp}
	c.cacheBytes += len(data)
}

func decodeOutfitObject(data []byte, output any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(output); err != nil {
		return errors.New("EDSM returned an unexpected response; check the system name or try again later")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("EDSM returned more than one JSON value")
	}
	return nil
}

func boundedOutfitArray[T any](data json.RawMessage, limit int) ([]T, error) {
	if len(data) == 0 {
		return nil, errors.New("EDSM response is missing the requested data")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return []T{}, nil
	}
	d := json.NewDecoder(bytes.NewReader(data))
	start, err := d.Token()
	if err != nil || start != json.Delim('[') {
		return nil, errors.New("EDSM returned an invalid data list")
	}
	result := make([]T, 0)
	for d.More() {
		if len(result) >= limit {
			return nil, errors.New("EDSM returned too many records")
		}
		var entry T
		if err := d.Decode(&entry); err != nil {
			return nil, errors.New("EDSM returned an invalid data record; try again later")
		}
		result = append(result, entry)
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	return result, nil
}

func outfitText(value string, limit int) string {
	value = strings.TrimSpace(safeText(value))
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func outfitInputText(value string) string {
	value = safeText(value)
	if runes := []rune(value); len(runes) > 128 {
		return string(runes[:128])
	}
	return value
}

func outfitTimestamp(value string) time.Time {
	for _, layout := range []string{time.RFC3339, time.DateTime} {
		if stamp, err := time.Parse(layout, value); err == nil {
			return stamp.UTC()
		}
	}
	return time.Time{}
}

func (c *outfittingClient) stations(ctx context.Context, input string, refresh bool) (outfitStationsResult, error) {
	system, err := outfitSystemName(input)
	if err != nil {
		return outfitStationsResult{}, err
	}
	values := url.Values{"systemName": {system}}
	path := "/api-system-v1/stations"
	data, fetched, cached, err := c.get(ctx, path, values, refresh)
	if err != nil {
		return outfitStationsResult{}, err
	}
	var root struct {
		Name     string
		Stations json.RawMessage
	}
	if err := decodeOutfitObject(data, &root); err != nil {
		return outfitStationsResult{}, err
	}
	if root.Name == "" {
		return outfitStationsResult{}, errors.New("system not found or EDSM has no system data")
	}
	if !strings.EqualFold(root.Name, system) {
		return outfitStationsResult{}, errors.New("EDSM returned a different system; check the system name")
	}
	type record struct {
		ID                uint64
		MarketID          uint64
		Name, Type        string
		DistanceToArrival *float64
		HaveOutfitting    bool
		UpdateTime        struct{ Outfitting string }
	}
	records, err := boundedOutfitArray[record](root.Stations, 4000)
	if err != nil {
		return outfitStationsResult{}, err
	}
	result := outfitStationsResult{System: outfitText(root.Name, 128), Stations: []outfitStation{}, Retrieved: fetched, Cached: cached}
	seen := map[string]bool{}
	for i, r := range records {
		if i%32 == 0 && ctx.Err() != nil {
			return outfitStationsResult{}, ctx.Err()
		}
		name := outfitText(r.Name, 128)
		if name == "" {
			continue
		}
		key := fmt.Sprintf("%s/%d/%d/%s", result.System, r.ID, r.MarketID, name)
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Stations = append(result.Stations, outfitStation{Key: key, System: result.System, MarketID: r.MarketID, Name: name, Type: outfitText(r.Type, 64),
			Distance: r.DistanceToArrival, HaveOutfitting: r.HaveOutfitting, Updated: outfitTimestamp(r.UpdateTime.Outfitting)})
	}
	sort.SliceStable(result.Stations, func(i, j int) bool {
		a, b := result.Stations[i], result.Stations[j]
		if a.HaveOutfitting != b.HaveOutfitting {
			return a.HaveOutfitting
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	if !cached {
		c.remember(path, values, data, fetched)
	}
	return result, nil
}

var outfitClassRating = regexp.MustCompile(`^(\d{1,2})([A-I])\s+`)
var outfitCategories = []string{"All types", "Internal modules", "Hardpoints", "Utility mounts", "Bulkheads", "Other"}

func classifyOutfitModule(id, name string) outfitModule {
	m := outfitModule{ID: outfitText(id, 160), Name: outfitText(name, 180), Category: "Other"}
	if m.Name == "" {
		m.Name = m.ID
	}
	code := strings.ToLower(m.ID)
	switch {
	case strings.Contains(code, "_armour_"):
		m.Category = "Bulkheads"
		m.Ship = shipDisplayName(strings.SplitN(code, "_armour_", 2)[0])
	case strings.HasPrefix(code, "hpt_"):
		m.Category = "Hardpoints"
		if strings.Contains(code, "_tiny") {
			m.Category = "Utility mounts"
		}
	case strings.HasPrefix(code, "int_"):
		m.Category = "Internal modules"
	}
	if match := outfitClassRating.FindStringSubmatch(m.Name); len(match) == 3 {
		class, _ := strconv.Atoi(match[1])
		m.Class = &class
		m.Rating = match[2]
	}
	if strings.HasPrefix(code, "hpt_") && m.Class != nil && *m.Class == 0 {
		m.Category = "Utility mounts"
	}
	return m
}

func (c *outfittingClient) modules(ctx context.Context, station outfitStation, refresh bool) (outfitModulesResult, error) {
	values := url.Values{}
	if station.MarketID != 0 {
		values.Set("marketId", strconv.FormatUint(station.MarketID, 10))
	} else {
		values.Set("systemName", station.System)
		values.Set("stationName", station.Name)
	}
	path := "/api-system-v1/stations/outfitting"
	data, fetched, cached, err := c.get(ctx, path, values, refresh)
	if err != nil {
		return outfitModulesResult{}, err
	}
	var root struct {
		Name, SName string
		MarketID    uint64
		Outfitting  json.RawMessage
	}
	if err := decodeOutfitObject(data, &root); err != nil {
		return outfitModulesResult{}, err
	}
	if !strings.EqualFold(root.Name, station.System) || !strings.EqualFold(root.SName, station.Name) || station.MarketID != 0 && root.MarketID != station.MarketID {
		return outfitModulesResult{}, errors.New("station identity/location changed or data is unavailable; reload the system's stations")
	}
	type record struct{ ID, Name string }
	records, err := boundedOutfitArray[record](root.Outfitting, 6000)
	if err != nil {
		return outfitModulesResult{}, err
	}
	result := outfitModulesResult{Modules: []outfitModule{}, Retrieved: fetched, Cached: cached}
	seen := map[string]bool{}
	for i, r := range records {
		if i%32 == 0 && ctx.Err() != nil {
			return outfitModulesResult{}, ctx.Err()
		}
		m := classifyOutfitModule(r.ID, r.Name)
		key := m.ID
		if key == "" {
			key = m.Name
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result.Modules = append(result.Modules, m)
	}
	sort.SliceStable(result.Modules, func(i, j int) bool {
		a, b := result.Modules[i], result.Modules[j]
		if a.Name == b.Name {
			return a.Ship < b.Ship
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	if !cached {
		c.remember(path, values, data, fetched)
	}
	return result, nil
}

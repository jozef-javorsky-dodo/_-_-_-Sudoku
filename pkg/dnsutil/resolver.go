/*
Copyright (C) 2026 by saba <contact me via issue>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
*/
package dnsutil

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// lookupIPFunc abstracts DNS lookups for easier testing.
type lookupIPFunc func(ctx context.Context, network, host string) ([]net.IP, error)

type cacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
}

type resolver struct {
	mu       sync.RWMutex
	cache    map[string]cacheEntry
	ttl      time.Duration
	lookupFn lookupIPFunc
}

func newResolver(ttl time.Duration, fn lookupIPFunc) *resolver {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if fn == nil {
		fn = func(ctx context.Context, network, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, network, host)
		}
	}
	return &resolver{
		cache:    make(map[string]cacheEntry),
		ttl:      ttl,
		lookupFn: fn,
	}
}

var defaultResolver = newResolver(10*time.Minute, nil)

// ResolveWithCache resolves addr (host:port) into ip:port using
// concurrent DNS lookups (IPv4/IPv6) and optimistic caching.
//
// Behavior:
//   - If host is already an IP, returns addr directly.
//   - If a fresh cache entry exists, returns it without DNS queries.
//   - If cache is stale and DNS fails, falls back to stale IP (optimistic cache).
//   - DNS lookups for IPv4/IPv6 are performed concurrently.
func ResolveWithCache(ctx context.Context, addr string) (string, error) {
	return defaultResolver.Resolve(ctx, addr)
}

// LookupIPsWithCache returns the resolved IPs for host using an optimistic cache.
//
// Behavior:
//   - If host is already an IP literal, returns it.
//   - If cache is fresh, returns cached results.
//   - If cache is stale and DNS fails, returns stale results.
//   - If there is no cache and DNS fails, returns an error.
func LookupIPsWithCache(ctx context.Context, host string) ([]net.IP, error) {
	return defaultResolver.LookupIPs(ctx, host)
}

// Resolve performs the actual resolution logic on a resolver instance.
func (r *resolver) Resolve(ctx context.Context, addr string) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("empty address")
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid address %q: %w", addr, err)
	}

	// If already an IP literal, no DNS is needed.
	if ip := net.ParseIP(host); ip != nil {
		return addr, nil
	}

	ips, err := r.LookupIPs(ctx, host)
	if err != nil {
		return "", err
	}

	selected := pickPreferredIP(ips)
	if selected == nil {
		return "", fmt.Errorf("no usable ip found for host %s", host)
	}
	return net.JoinHostPort(selected.String(), port), nil
}

func (r *resolver) LookupIPs(ctx context.Context, host string) ([]net.IP, error) {
	hostKey, hostIP := normalizeHost(host)
	if hostIP != nil {
		return []net.IP{hostIP}, nil
	}
	if hostKey == "" {
		return nil, fmt.Errorf("empty host")
	}

	now := time.Now()
	cachedIPs, expired := r.lookup(hostKey, now)

	if len(cachedIPs) > 0 && !expired {
		return copyIPs(cachedIPs), nil
	}

	ips, err := r.lookupConcurrently(ctx, hostKey)
	if err != nil {
		if len(cachedIPs) > 0 {
			return copyIPs(cachedIPs), nil
		}
		return nil, fmt.Errorf("dns lookup failed for %s: %w", hostKey, err)
	}

	ips = normalizeIPs(ips)
	if len(ips) == 0 {
		if len(cachedIPs) > 0 {
			return copyIPs(cachedIPs), nil
		}
		return nil, fmt.Errorf("no usable ip found for host %s", hostKey)
	}

	r.store(hostKey, ips, now)
	return copyIPs(ips), nil
}

func (r *resolver) lookup(hostKey string, now time.Time) ([]net.IP, bool) {
	r.mu.RLock()
	entry, ok := r.cache[hostKey]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		return entry.ips, true
	}
	return entry.ips, false
}

func (r *resolver) store(hostKey string, ips []net.IP, now time.Time) {
	ips = normalizeIPs(ips)
	if len(ips) == 0 {
		return
	}
	r.mu.Lock()
	r.cache[hostKey] = cacheEntry{
		ips:       copyIPs(ips),
		expiresAt: now.Add(r.ttl),
	}
	r.mu.Unlock()
}

func (r *resolver) lookupConcurrently(ctx context.Context, host string) ([]net.IP, error) {
	type result struct {
		ips []net.IP
		err error
	}

	networks := []string{"ip4", "ip6"}
	ch := make(chan result, len(networks))

	var wg sync.WaitGroup
	for _, network := range networks {
		network := network
		wg.Add(1)
		go func() {
			defer wg.Done()
			ips, err := r.lookupFn(ctx, network, host)
			select {
			case ch <- result{ips: ips, err: err}:
			case <-ctx.Done():
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var allIPs []net.IP
	var firstErr error

	for res := range ch {
		if res.err == nil && len(res.ips) > 0 {
			allIPs = append(allIPs, res.ips...)
		} else if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}

	if len(allIPs) == 0 {
		if firstErr == nil {
			firstErr = fmt.Errorf("no ip records found")
		}
		return nil, firstErr
	}

	return allIPs, nil
}

func normalizeHost(host string) (string, net.IP) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", nil
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSuffix(host, ".")
	host = strings.ToLower(host)
	if ip := net.ParseIP(host); ip != nil {
		return host, ip
	}
	return host, nil
}

func normalizeIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	out := ips[:0]
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			out = append(out, ip4)
			continue
		}
		if ip16 := ip.To16(); ip16 != nil {
			out = append(out, ip16)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return preferIPv4First(out)
}

func preferIPv4First(ips []net.IP) []net.IP {
	v4 := make([]net.IP, 0, len(ips))
	v6 := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	return append(v4, v6...)
}

func pickPreferredIP(ips []net.IP) net.IP {
	ips = normalizeIPs(append([]net.IP(nil), ips...))
	if len(ips) == 0 {
		return nil
	}
	return ips[0]
}

func copyIPs(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		out = append(out, append(net.IP(nil), ip...))
	}
	return out
}

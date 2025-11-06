package ssrf

import (
    "errors"
    "net"
    "net/netip"
    "net/url"
)

var (
    ipv4Private = []netip.Prefix{
        netip.MustParsePrefix("127.0.0.0/8"),
        netip.MustParsePrefix("10.0.0.0/8"),
        netip.MustParsePrefix("172.16.0.0/12"),
        netip.MustParsePrefix("192.168.0.0/16"),
        netip.MustParsePrefix("169.254.0.0/16"),
        netip.MustParsePrefix("0.0.0.0/8"),
        netip.MustParsePrefix("192.0.0.0/24"),
        netip.MustParsePrefix("100.64.0.0/10"),
        netip.MustParsePrefix("198.18.0.0/15"),
        netip.MustParsePrefix("224.0.0.0/4"),
        netip.MustParsePrefix("240.0.0.0/4"),
    }
    ipv6Private = []netip.Prefix{
        netip.MustParsePrefix("::1/128"),
        netip.MustParsePrefix("::/128"),
        netip.MustParsePrefix("fe80::/10"),
        netip.MustParsePrefix("fc00::/7"),
        netip.MustParsePrefix("::ffff:0:0/96"),
        netip.MustParsePrefix("2001:db8::/32"),
        netip.MustParsePrefix("ff00::/8"),
    }
)

func isBlockedIP(ip netip.Addr) bool {
    if ip.IsLoopback() { return true }
    if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { return true }
    for _, p := range ipv4Private {
        if p.Contains(ip) { return true }
    }
    for _, p := range ipv6Private {
        if p.Contains(ip) { return true }
    }
    return false
}

// ResolveAndPin resolves the host and returns an allowed IP and the original host.
func ResolveAndPin(u *url.URL) (netip.Addr, string, error) {
    host := u.Hostname()
    if host == "" {
        return netip.Addr{}, "", errors.New("empty host")
    }
    ips, err := net.LookupIP(host)
    if err != nil {
        return netip.Addr{}, "", err
    }
    for _, ip := range ips {
        addr, ok := netip.AddrFromSlice(ip)
        if !ok { continue }
        if isBlockedIP(addr) { continue }
        return addr, host, nil
    }
    return netip.Addr{}, host, errors.New("no allowed ip for host")
}


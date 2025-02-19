package SunnyProxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/qtgolang/SunnyNet/src/crypto/tls"
	"github.com/qtgolang/SunnyNet/src/dns"
	"golang.org/x/net/proxy"
	"net"
	"net/url"
	"strings"
	"time"
)

var dnsConfig = &tls.Config{
	ClientSessionCache: tls.NewLRUClientSessionCache(32),
	InsecureSkipVerify: true,
}
var invalidProxy = fmt.Errorf("invalid host")

type Proxy struct {
	*url.URL
	timeout  time.Duration
	Regexp   func(Host string) bool
	DialAddr string
}

func ParseProxy(u string, timeout ...int) (*Proxy, error) {
	var err error
	p := &Proxy{}
	p.URL, err = url.Parse(u)
	if err != nil {
		return nil, err
	}
	if p.URL == nil {
		return nil, invalidProxy
	}
	Scheme := strings.ToLower(p.URL.Scheme)
	if Scheme != "http" && Scheme != "https" && Scheme != "socket" && Scheme != "sock" && Scheme != "socket5" && Scheme != "socks5" && Scheme != "socks" {
		return nil, fmt.Errorf("invalid scheme: %s", p.URL.Scheme)
	}
	if Scheme == "socket" || Scheme == "sock" || Scheme == "socket5" || Scheme == "socks5" || Scheme == "socks" {
		p.URL.Scheme = "socks5"
	}
	if len(p.Host) < 3 {
		return nil, invalidProxy
	}

	p.timeout = 30 * time.Second
	if len(timeout) > 0 {
		p.timeout = time.Duration(timeout[0]) * time.Millisecond
	}
	return p, err
}
func (p *Proxy) IsSocksType() bool {
	if p == nil {
		return false
	}
	if p.URL == nil {
		return false
	}
	return p.URL.Scheme == "socks5"
}
func (p *Proxy) String() string {
	if p == nil {
		return ""
	}
	if p.URL == nil {
		return ""
	}
	return p.URL.String()
}
func (p *Proxy) User() string {
	if p == nil {
		return ""
	}
	if p.URL == nil {
		return ""
	}
	if p.URL.User == nil {
		return ""
	}
	return p.URL.User.Username()
}
func (p *Proxy) Pass() string {
	if p == nil {
		return ""
	}
	if p.URL == nil {
		return ""
	}
	if p.URL.User == nil {
		return ""
	}
	pass, _ := p.URL.User.Password()
	return pass
}
func (p *Proxy) Clone() *Proxy {
	if p == nil {
		return nil
	}
	if p.URL == nil {
		return nil
	}
	if len(p.Host) < 3 {
		return nil
	}
	n := &Proxy{}

	n.URL, _ = url.Parse(p.URL.String())
	n.timeout = p.timeout
	n.Regexp = p.Regexp
	n.DialAddr = p.DialAddr
	return n
}
func (p *Proxy) SetTimeout(d time.Duration) {
	if p == nil {
		return
	}
	p.timeout = d
	return
}
func (p *Proxy) getTimeout() time.Duration {
	if p == nil || p.timeout == 0 {
		return 15 * time.Second
	}
	return p.timeout
}
func (p *Proxy) getSocksAuth() *proxy.Auth {
	if p.User() == "" {
		return nil
	}
	return &proxy.Auth{
		User:     p.User(),
		Password: p.Pass(),
	}
}
func (p *Proxy) DialWithTimeout(network, addr string, Timeout time.Duration) (net.Conn, error) {
	pp := p.Clone()
	if pp == nil {
		pp = &Proxy{}
	}
	pp.timeout = Timeout
	return pp.Dial(network, addr)
}
func (p *Proxy) Dial(network, addr string) (net.Conn, error) {
	var directDialer = direct{timeout: p.getTimeout()}
	Host, _, _ := net.SplitHostPort(addr)
	if p == nil {
		a, e := directDialer.Dial(network, addr)
		return a, e
	}
	p.DialAddr = Host
	if p.URL == nil {
		a, e := directDialer.Dial(network, addr)
		if a != nil {
			p.DialAddr = a.RemoteAddr().String()
		}
		return a, e
	}
	if p.Regexp != nil {
		if Host != "" && p.Regexp(Host) {
			a, e := directDialer.Dial(network, addr)
			if a != nil {
				p.DialAddr = a.RemoteAddr().String()
			}
			return a, e
		}
	}

	proxyAddr := p.Host
	proxyHost, proxyPort, e := net.SplitHostPort(p.Host)

	var ips []net.IP
	var first net.IP
	if e == nil {
		if net.ParseIP(proxyHost) == nil {
			ips, _ = dns.LookupIP(proxyHost, "", nil)
			first = dns.GetFirstIP(proxyHost, "")
		}
	}
	var conn net.Conn
	if p.IsSocksType() {
		d, err1 := proxy.SOCKS5("tcp", proxyAddr, p.getSocksAuth(), directDialer)
		if err1 != nil {
			return nil, err1
		}
		if first != nil {
			conn, e = d.Dial(network, FormatIP(first, proxyPort))
			if conn != nil {
				p.DialAddr = FormatIP(first, proxyPort)
				return conn, e
			}
		}
		for _, ip := range ips {
			if ip != nil {
				conn, e = d.Dial(network, FormatIP(ip, proxyPort))
				if conn != nil {
					p.DialAddr = FormatIP(ip, proxyPort)
					dns.SetFirstIP(proxyHost, "", ip)
					return conn, e
				}
			}
		}
		conn, e = d.Dial(network, addr)
		if conn != nil {
			p.DialAddr = addr
		}
		return conn, e
	}

	if first != nil {
		p.DialAddr = FormatIP(first, proxyPort)
		conn, e = directDialer.Dial(network, p.DialAddr)
	}
	if conn == nil {
		for _, ip := range ips {
			if ip != nil {
				p.DialAddr = FormatIP(ip, proxyPort)
				conn, e = directDialer.Dial(network, p.DialAddr)
				if conn != nil {
					dns.SetFirstIP(proxyHost, "", ip)
					break
				}
			}
		}
		if conn == nil {
			p.DialAddr = proxyAddr
			conn, e = directDialer.Dial(network, proxyAddr)
		}
	}
	if e != nil {
		return nil, e
	}
	us := ""
	if p.User() != "" {
		us = "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(p.User()+":"+p.Pass())) + "\r\n"
	}
	_, e = conn.Write([]byte("CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n" + us + "\r\n"))
	if e != nil {
		return nil, e
	}
	b := make([]byte, 128)
	n, er := conn.Read(b)
	if n < 13 {
		_ = conn.Close()
		return nil, er
	}
	if string(b[:12]) != "HTTP/1.1 200" {
		return nil, fmt.Errorf(string(b))
	}
	return conn, er
}

type direct struct {
	timeout time.Duration
}

func (ps direct) Dial(network, addr string) (net.Conn, error) {
	var m net.Dialer
	m.Timeout = ps.timeout
	return m.Dial(network, addr)
}
func (ps direct) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var m net.Dialer
	m.Timeout = ps.timeout
	return m.DialContext(ctx, network, addr)
}
func FormatIP(ip net.IP, port string) string {
	if ip.To4() != nil {
		return fmt.Sprintf("%s:%s", ip.String(), port)
	}
	return fmt.Sprintf("[%s]:%s", ip.String(), port)
}

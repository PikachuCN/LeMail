package smtpdebug

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/PikachuCN/LeMail/internal/config"
)

type DNSIssue struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type DNSAddress struct {
	IP      string   `json:"ip"`
	Version string   `json:"version"`
	Flags   []string `json:"flags,omitempty"`
}

type DNSMX struct {
	Host       string       `json:"host"`
	Preference uint16       `json:"preference"`
	IsIP       bool         `json:"isIP"`
	Addresses  []DNSAddress `json:"addresses,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type DNSDomainReport struct {
	Domain    string       `json:"domain"`
	Addresses []DNSAddress `json:"addresses,omitempty"`
	MX        []DNSMX      `json:"mx,omitempty"`
	Issues    []DNSIssue   `json:"issues,omitempty"`
}

type DNSReport struct {
	CheckedAt time.Time         `json:"checkedAt"`
	SMTPAddr  string            `json:"smtpAddr"`
	SMTPPort  string            `json:"smtpPort,omitempty"`
	Domains   []DNSDomainReport `json:"domains"`
	Issues    []DNSIssue        `json:"issues,omitempty"`
}

func CheckDNS(cfg config.Config) DNSReport {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report := DNSReport{
		CheckedAt: time.Now().UTC(),
		SMTPAddr:  cfg.SMTP.Addr,
		SMTPPort:  smtpPort(cfg.SMTP.Addr),
		Domains:   make([]DNSDomainReport, 0, len(cfg.Mail.Domains)),
	}
	if report.SMTPPort == "" {
		report.Issues = append(report.Issues, DNSIssue{Level: "warning", Message: "无法从 smtp.addr 判断监听端口，请确认公网 SMTP 使用 25 端口"})
	} else if report.SMTPPort != "25" {
		report.Issues = append(report.Issues, DNSIssue{Level: "warning", Message: fmt.Sprintf("当前 SMTP 监听端口是 %s，公网收信通常需要 25 端口", report.SMTPPort)})
	}
	for _, domain := range cfg.Mail.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		report.Domains = append(report.Domains, checkDomain(ctx, domain))
	}
	return report
}

func checkDomain(ctx context.Context, domain string) DNSDomainReport {
	item := DNSDomainReport{Domain: domain}
	item.Addresses = lookupAddresses(ctx, domain)
	for _, address := range item.Addresses {
		for _, flag := range address.Flags {
			if flag != "" {
				item.Issues = append(item.Issues, DNSIssue{Level: "warning", Message: fmt.Sprintf("域名 %s 解析到特殊地址 %s（%s）", domain, address.IP, flag)})
			}
		}
	}
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		item.Issues = append(item.Issues, DNSIssue{Level: "warning", Message: fmt.Sprintf("查询 MX 失败：%v", err)})
		return item
	}
	if len(mxRecords) == 0 {
		item.Issues = append(item.Issues, DNSIssue{Level: "warning", Message: "没有 MX 记录，部分发信方不会回退到 A/AAAA 投递"})
		return item
	}
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(strings.TrimSpace(mx.Host), ".")
		record := DNSMX{Host: host, Preference: mx.Pref, IsIP: net.ParseIP(host) != nil}
		if record.IsIP {
			item.Issues = append(item.Issues, DNSIssue{Level: "error", Message: fmt.Sprintf("MX %s 直接指向 IP，建议改为 mx.%s 这类主机名", host, domain)})
		}
		if ip := net.ParseIP(host); ip != nil {
			record.Addresses = []DNSAddress{{IP: host, Version: ipVersion(ip), Flags: specialIPFlags(host)}}
		} else {
			record.Addresses = lookupAddresses(ctx, host)
		}
		if len(record.Addresses) == 0 {
			record.Error = "没有查询到 A/AAAA 地址"
			item.Issues = append(item.Issues, DNSIssue{Level: "error", Message: fmt.Sprintf("MX 主机 %s 没有可用 A/AAAA 地址", host)})
		}
		for _, address := range record.Addresses {
			for _, flag := range address.Flags {
				if flag != "" {
					item.Issues = append(item.Issues, DNSIssue{Level: "warning", Message: fmt.Sprintf("MX 主机 %s 解析到特殊地址 %s（%s）", host, address.IP, flag)})
				}
			}
		}
		item.MX = append(item.MX, record)
	}
	return item
}

func lookupAddresses(ctx context.Context, host string) []DNSAddress {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	items := make([]DNSAddress, 0, len(ips))
	seen := map[string]struct{}{}
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip == nil {
			continue
		}
		value := ip.String()
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, DNSAddress{IP: value, Version: ipVersion(ip), Flags: specialIPFlags(value)})
	}
	return items
}

func smtpPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 || idx == len(addr)-1 {
		return ""
	}
	port := addr[idx+1:]
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	return port
}

func ipVersion(ip net.IP) string {
	if ip.To4() != nil {
		return "A"
	}
	return "AAAA"
}

func specialIPFlags(value string) []string {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return nil
	}
	var flags []string
	switch {
	case addr.IsLoopback():
		flags = append(flags, "loopback")
	case addr.IsPrivate():
		flags = append(flags, "private")
	case addr.IsLinkLocalUnicast():
		flags = append(flags, "link-local")
	case addr.IsMulticast():
		flags = append(flags, "multicast")
	case addr.IsUnspecified():
		flags = append(flags, "unspecified")
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(addr) {
			flags = append(flags, prefix.String())
		}
	}
	return flags
}

var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

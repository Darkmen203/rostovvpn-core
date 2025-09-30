package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// DNSParsed — результат парсинга, если вдруг понадобится ещё где-то.
type DNSParsed struct {
	Scheme string // "udp", "https", "tls", "quic", "tcp", "local", "system", "rcode"
	Host   string // хост или IP (без порта)
	Port   int    // 53 по умолчанию для UDP/TCP; 853 для TLS; 443 для HTTPS/QUIC
	Path   string // для DoH ("/dns-query" и т.п.)
	Raw    string // исходная строка
	IsIP   bool
}

// defaults по схемам
func defaultPortForScheme(s string) int {
	switch s {
	case "udp", "tcp":
		return 53
	case "tls":
		return 853
	case "https", "quic":
		return 443
	default:
		return 0
	}
}

func isSchemeKnown(s string) bool {
	switch s {
	case "udp", "tcp", "tls", "https", "quic", "local", "system", "rcode":
		return true
	default:
		return false
	}
}

// ParseDNSAddr нормализует любые входные строки в DNSParsed.
func ParseDNSAddr(input string) (DNSParsed, error) {
	s := strings.TrimSpace(input)
	out := DNSParsed{Raw: s}

	// Спец-значения:
	if s == "local" || s == "system" || strings.HasPrefix(s, "rcode://") {
		out.Scheme = strings.SplitN(s, "://", 2)[0] // "local", "system", "rcode"
		if out.Scheme == "rcode" {
			out.Host = strings.TrimPrefix(s, "rcode://") // success/… (sing-box сам поймёт)
		}
		return out, nil
	}

	// Если указана явная схема — используем url.Parse
	if i := strings.Index(s, "://"); i > 0 {
		u, err := url.Parse(s)
		if err != nil {
			return out, fmt.Errorf("parse dns uri: %w", err)
		}
		out.Scheme = strings.ToLower(u.Scheme)
		if !isSchemeKnown(out.Scheme) {
			return out, fmt.Errorf("unknown dns scheme: %s", out.Scheme)
		}
		host := u.Host
		// host может содержать :port
		h, p, err := net.SplitHostPort(host)
		if err == nil {
			out.Host = h
			if port, err := strconv.Atoi(p); err == nil {
				out.Port = port
			}
		} else {
			out.Host = host
		}
		out.IsIP = net.ParseIP(out.Host) != nil
		out.Path = u.EscapedPath()
		if out.Port == 0 {
			out.Port = defaultPortForScheme(out.Scheme)
		}
		return out, nil
	}

	// Схемы нет: это либо "ip", либо "ip:port", либо домен (без схемы)
	// Пытаемся вычленить порт
	host := s
	if h, p, err := net.SplitHostPort(s); err == nil {
		host = h
		if port, err := strconv.Atoi(p); err == nil {
			out.Port = port
		}
	}
	out.Host = host
	out.IsIP = net.ParseIP(out.Host) != nil

	// По умолчанию считаем UDP (sing-box best practice)
	out.Scheme = "udp"
	if out.Port == 0 {
		out.Port = 53
	}
	return out, nil
}

// BuildDNSServer — собирает объект сервера в каноничном для sing-box виде (через address URI).
// tag            — желаемый tag
// addr           — то, что пришло из настроек (любой формат: "udp://8.8.8.8:55", "8.8.8.8", "https://...")
// detour         — если нужно (например, "direct"), "" — пропустить
// addressResolver — если хочешь проставить address_resolver (ТОЛЬКО для доменов, не для IP)
func BuildDNSServer(tag, addr, detour, addressResolver string) (map[string]any, error) {
	p, err := ParseDNSAddr(addr)
	if err != nil {
		return nil, err
	}

	obj := map[string]any{
		"tag": tag,
	}

	switch p.Scheme {
	case "local", "system":
		obj["address"] = p.Scheme // "local" или "system"
		if detour != "" {
			obj["detour"] = detour
		}
	case "rcode":
		// rcode://success и пр.
		obj["address"] = "rcode://" + p.Host
	case "udp", "tcp", "tls", "https", "quic":
		// Собираем URI
		hostport := p.Host
		if p.Port != 0 {
			hostport = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		}
		uri := p.Scheme + "://" + hostport
		if p.Path != "" && (p.Scheme == "https" || p.Scheme == "quic") {
			// для DoH/DoQ можно передать путь
			if !strings.HasPrefix(p.Path, "/") {
				uri += "/" + p.Path
			} else {
				uri += p.Path
			}
		}
		obj["address"] = uri

		// address_resolver имеет смысл только если host — ДОМЕН
		if addressResolver != "" && !p.IsIP {
			obj["address_resolver"] = addressResolver
		}
		if detour != "" {
			obj["detour"] = detour
		}
	default:
		return nil, fmt.Errorf("unsupported dns scheme: %s", p.Scheme)
	}

	return obj, nil
}

// (Опционально) UDP-структурный вариант: type/server/server_port.
// Использовать ТОЛЬКО если тебе это действительно нужно, и ОСОБЕННО аккуратно —
// такой формат для DNS-серверов не везде поддерживается так же стабильно, как URI.
func BuildDNSServerUDPStructured(tag, addr, detour string) (map[string]any, error) {
	p, err := ParseDNSAddr(addr)
	if err != nil {
		return nil, err
	}
	if p.Scheme != "udp" {
		return nil, fmt.Errorf("structured DNS builder supports only udp scheme, got %s", p.Scheme)
	}
	obj := map[string]any{
		"type": "udp",
		"tag":  tag,
		// sing-box традиционно ожидает URI в "address", но если очень нужно:
		"server": p.Host,
	}
	if p.Port != 0 && p.Port != 53 {
		obj["server_port"] = p.Port
	}
	if detour != "" {
		obj["detour"] = detour
	}
	return obj, nil
}

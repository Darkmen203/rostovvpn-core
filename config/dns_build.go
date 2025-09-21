package config

import (
	"fmt"
	"net/url"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
)

// parseHostPort("host[:port]", def) -> (server, port)
func parseHostPort(host string, def uint16) (string, uint16) {
	server := host
	port := def
	if hp := M.ParseSocksaddr(host); hp.IsValid() {
		server = hp.AddrString()
		if hp.Port != 0 {
			port = hp.Port
		}
	}
	return server, port
}

func newDNSServer(tag, address, resolver string, strategy option.DomainStrategy, detour string) option.DNSServerOptions {
	addr := strings.TrimSpace(address)
	if addr == "" {
		// подстраховка — пусть будет legacy, он безопасно пережуёт пустяки
		return legacyDNSServer(tag, address, resolver, strategy, detour)
	}

	u, _ := url.Parse(address)
	scheme := strings.ToLower(u.Scheme)
	host := u.Host
	fmt.Println("[newDNSServer] host =\n\n\n\n", scheme, "\n\n\n [newDNSServer]")

	path := u.Path
	if strings.TrimSpace(host) == "" &&
		address == C.DNSTypeLocal {
		// fallback: не плодим пустой server
		// return legacyDNSServer(tag, address, resolver, strategy, detour)
		scheme = C.DNSTypeLocal;
	}
	if strings.TrimSpace(host) == "" &&
		scheme != C.DNSTypeLocal && scheme != C.DNSTypeHosts && scheme != C.DNSTypeFakeIP {
		// fallback: не плодим пустой server
		return legacyDNSServer(tag, address, resolver, strategy, detour)
	}

	// если схемы нет и это не спец-слово — считаем UDP "host[:port]"
	if scheme == "" {
		switch strings.ToLower(address) {
		case C.DNSTypeLocal, C.DNSTypeFakeIP, C.DNSTypeHosts: // только токены, которые реально есть
			scheme = strings.ToLower(address)
		default:
			scheme = C.DNSTypeUDP
			host = address
		}
	}

	// общая «корзина» опций для текущего сервера
	obj := map[string]any{}

	// dial-поля
	if detour != "" && detour != "direct" && detour != "dns-trick-direct" && detour != "dns-direct" && detour != "local" {
		obj["detour"] = detour
	}

	det := detour
	if det == "direct" || det == "dns-trick-direct" || det == "dns-direct" || det == "local" {
		det = ""
	}

	var dr *option.DomainResolveOptions
	if resolver != "" || strategy != 0 {
		dr = &option.DomainResolveOptions{Server: resolver}
		if strategy != 0 {
			dr.Strategy = strategy
		}
	}
	// detour to an empty direct outbound makes no sense
	fmt.Println("[newDNSServer] scheme =\n\n\n\n", scheme, "\n\n\n [newDNSServer]")

	switch scheme {
	case C.DNSTypeLocal:
		o := option.DNSServerOptions{Type: C.DNSTypeLocal, Tag: tag}
		remoteOptions := option.RemoteDNSServerOptions{
			RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{
				DialerOptions: option.DialerOptions{
					Detour: "direct",
					DomainResolver: &option.DomainResolveOptions{
						Server:   resolver,
						Strategy: strategy,
					},
				},
			},
			LegacyAddressResolver: resolver,
			LegacyAddressStrategy: strategy,
		}
		o.Options = &option.LocalDNSServerOptions{
			RawLocalDNSServerOptions: remoteOptions.RawLocalDNSServerOptions,
		}
		fmt.Println("[newDNSServer] return local =\n\n\n\n", o, "\n\n\n [newDNSServer]")

		// local — без адреса; detour/domain_resolver можно оставить (ядро проигнорит)
		return o

	case C.DNSTypeTCP, C.DNSTypeUDP:
		server, port := parseHostPort(host, 53)
		obj["server"] = server
		if port != 53 {
			obj["server_port"] = port
		}
		opts := &option.RemoteDNSServerOptions{
			RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{
				DialerOptions: option.DialerOptions{
					Detour:         det,
					DomainResolver: dr,
				},
			},
			DNSServerAddressOptions: option.DNSServerAddressOptions{
				Server:     server,
				ServerPort: port,
			},
		}
		fmt.Println("[newDNSServer] return tcp or udp =\n\n\n\n", option.DNSServerOptions{Type: scheme, Tag: tag, Options: opts}, "\n\n\n [newDNSServer]")
		return option.DNSServerOptions{Type: scheme, Tag: tag, Options: opts}

	case C.DNSTypeTLS, C.DNSTypeQUIC:
		def := uint16(853)
		server, port := parseHostPort(host, def)
		obj["server"] = server
		if port != def {
			obj["server_port"] = port
		}
		// при желании сюда позже можно добавить obj["tls"] = { ... }
		return option.DNSServerOptions{Type: scheme, Tag: tag, Options: obj}

	case C.DNSTypeHTTPS, C.DNSTypeHTTP3: // https / h3
		o := option.DNSServerOptions{Type: scheme, Tag: tag}
		def := uint16(443)
		server, port := parseHostPort(host, def)
		obj["server"] = server
		if port != def {
			obj["server_port"] = port
		}
		if path != "" {
			obj["path"] = path
		}
		fmt.Println("[newDNSServer] !!! path=\n\n", path, "\n !!! [newDNSServer]")

		remoteOptions := option.RemoteDNSServerOptions{
			RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{
				DialerOptions: option.DialerOptions{
					Detour:         det,
					DomainResolver: dr,
				},
				// Legacy:              true,
				// LegacyStrategy:      strategy,
				// LegacyDefaultDialer: det == "",
			},
			LegacyAddressResolver: resolver,
			LegacyAddressStrategy: strategy,
		}

		httpsOptions := option.RemoteHTTPSDNSServerOptions{
			RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{
				RemoteDNSServerOptions: remoteOptions,
			},
		}
		o.Options = &httpsOptions
		serverAddr := M.ParseSocksaddr(address)
		fmt.Println("[newDNSServer] serverAddr= \n\n\n", serverAddr, " !!! [newDNSServer]")
		httpsOptions.Server = host

		if serverAddr.Port != 0 && serverAddr.Port != 443 {
			httpsOptions.ServerPort = port
		}
		if path != "/dns-query" && path != "/" {
			httpsOptions.Path = path
		}
		fmt.Println("[newDNSServer] return https =\n\n\n\n", option.DNSServerOptions{
			Type:    scheme,
			Tag:     tag,
			Options: o.Options,
		}, "\n\n\n [newDNSServer]")

		return option.DNSServerOptions{
			Type:    scheme,
			Tag:     tag,
			Options: o.Options,
		}

	case C.DNSTypeDHCP: // dhcp://iface | dhcp://auto
		iface := host
		if strings.TrimSpace(iface) == "" {
			iface = "auto"
		}
		obj["interface"] = iface
		return option.DNSServerOptions{Type: C.DNSTypeDHCP, Tag: tag, Options: obj}

	case C.DNSTypeFakeIP:
		// при необходимости можно добавить inet4_range/inet6_range (как строки)
		return option.DNSServerOptions{Type: C.DNSTypeFakeIP, Tag: tag, Options: obj}

	case C.DNSTypeHosts:
		// этот тип ты используешь в applyStaticIPHosts через типизированную структуру — тут обычно не нужен
		return option.DNSServerOptions{Type: C.DNSTypeHosts, Tag: tag, Options: obj}
	}

	// по умолчанию — UDP
	server, port := parseHostPort(address, 53)
	obj["server"] = server
	if port != 53 {
		obj["server_port"] = port
	}
	fmt.Println("[newDNSServer] return =\n\n\n\n", option.DNSServerOptions{Type: C.DNSTypeUDP, Tag: tag, Options: obj}, "\n\n\n [newDNSServer]")
	return option.DNSServerOptions{Type: C.DNSTypeUDP, Tag: tag, Options: obj}
}

// ---- Массовая сборка + валидация ----

type DNSInput struct {
	Tag      string
	Address  string
	Resolver string
	Strategy option.DomainStrategy
	Detour   string
}

// BuildDNSServers валидирует и конвертирует список в []option.DNSServerOptions.
func BuildDNSServers(inputs []DNSInput) ([]option.DNSServerOptions, error) {
	out := make([]option.DNSServerOptions, 0, len(inputs))
	dup := map[string]struct{}{}

	for _, in := range inputs {
		if strings.TrimSpace(in.Tag) == "" {
			return nil, fmt.Errorf("dns: пустой tag недопустим")
		}
		if _, ok := dup[in.Tag]; ok {
			return nil, fmt.Errorf("dns: дублирующийся tag: %s", in.Tag)
		}
		dup[in.Tag] = struct{}{}

		addr := strings.TrimSpace(in.Address)
		if addr == "" {
			return nil, fmt.Errorf("dns: пустой address у tag=%s", in.Tag)
		}

		// Для DoH/H3 с доменным именем — желательно указать resolver (иначе некуда будет резолвить сам DoH-хост)
		u, _ := url.Parse(addr)
		scheme := strings.ToLower(u.Scheme)
		host := u.Host
		if (scheme == C.DNSTypeHTTPS || scheme == C.DNSTypeHTTP3) && host != "" {
			// грубая проверка: если host не выглядит как IPv4/IPv6 — считать доменом
			isIPish := func(h string) bool {
				for _, r := range h {
					if (r >= '0' && r <= '9') || r == '.' || r == ':' || r == '[' || r == ']' {
						continue
					}
					return false
				}
				return true
			}
			if !isIPish(host) && in.Resolver == "" {
				return nil, fmt.Errorf("dns[%s]: https/h3 с доменным именем лучше указывать с resolver", in.Tag)
			}
		}

		out = append(out, newDNSServer(in.Tag, addr, in.Resolver, in.Strategy, in.Detour))
	}

	return out, nil
}

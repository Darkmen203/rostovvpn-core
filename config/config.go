package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	dns "github.com/sagernet/sing-dns"
	singjson "github.com/sagernet/sing/common/json"
	badjson "github.com/sagernet/sing/common/json/badjson"
	badoption "github.com/sagernet/sing/common/json/badoption"
)

const (
	DNSRemoteTag       = "dns-remote"
	DNSLocalTag        = "dns-local"
	DNSDirectTag       = "dns-direct"
	DNSBlockTag        = "dns-block"
	DNSFakeTag         = "dns-fake"
	DNSBootstrapTag    = "dns-bootstrap"
	DNSTricksDirectTag = "dns-trick-direct"
	DNSWarpHostsTag    = "dns-warp-hosts"

	OutboundDirectTag         = "direct"
	OutboundBypassTag         = "bypass"
	OutboundBlockTag          = "block"
	OutboundSelectTag         = "select"
	OutboundURLTestTag        = "auto"
	OutboundDNSTag            = "dns-out"
	OutboundDirectFragmentTag = "direct-fragment"

	InboundTUNTag   = "tun-in"
	InboundMixedTag = "mixed-in"
	InboundDNSTag   = "dns-in"
)

var OutboundMainProxyTag = OutboundSelectTag

func normalizeDNSAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}

	low := strings.ToLower(addr)

	// спец-адреса пропускаем
	if low == "local" || low == "system" || low == "fakeip" || strings.HasPrefix(low, "rcode://") {
		return addr
	}

	// если схемы нет — считаем, что это udp
	if !strings.HasPrefix(low, "udp://") &&
		!strings.HasPrefix(low, "tcp://") &&
		!strings.HasPrefix(low, "tls://") &&
		!strings.HasPrefix(low, "https://") &&
		!strings.HasPrefix(low, "h3://") &&
		!strings.HasPrefix(low, "quic://") {
		addr = "udp://" + addr
		low = "udp://" + low
	}

	u, err := url.Parse(addr)
	if err != nil {
		return addr
	}
	host := u.Host
	hasPort := strings.LastIndex(host, ":") > strings.LastIndex(host, "]") // IPv6 с []
	if !hasPort {
		switch {
		case strings.HasPrefix(low, "udp://"), strings.HasPrefix(low, "tcp://"):
			u.Host = host + ":53"
		case strings.HasPrefix(low, "tls://"):
			u.Host = host + ":853"
		default: // https/h3/quic
			u.Host = host + ":443"
		}
	}

	// // ВАЖНО: для udp/tcp в legacy многие сборки хотят «host:port» без схемы.
	// if strings.HasPrefix(low, "udp://") || strings.HasPrefix(low, "tcp://") {
	// 	return u.Host
	// }
	return u.String()
}

func BuildConfigJson(configOpt RostovVPNOptions, input option.Options) (string, error) {
	options, err := BuildConfig(configOpt, input)
	if err != nil {
		return "", err
	}

	// ВАЖНО: кодируем через singjson, чтобы корректно инлайнить варианты Options
	raw, err := singjson.Marshal(options)
	if err != nil {
		return "", err
	}

	// Красивый вывод (не обязателен для libbox, но удобен для логов)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		// на случай, если вдруг indent не вышел — вернём компактный
		return string(raw), nil
	}
	return pretty.String(), nil
}

// TODO include selectors
func BuildConfig(opt RostovVPNOptions, input option.Options) (*option.Options, error) {
	// fmt.Printf("config options: %++v\n", opt)

	var options option.Options
	if opt.EnableFullConfig {
		options.Inbounds = input.Inbounds
		options.DNS = input.DNS
		options.Route = input.Route
		options.Experimental = input.Experimental
	} else {
		// даже если не full, всё равно уважим experimental из входного файла
		if input.Experimental != nil {
			options.Experimental = input.Experimental
		}
	}

	if !opt.EnableClashApi {
		if options.Experimental != nil {
			options.Experimental.ClashAPI = nil
		}
	}

	// fmt.Print("[BuildConfig] !!! input= \n", input, ",\n  !!! [BuildConfig] ")
	setClashAPI(&options, &opt)
	setLog(&options, &opt)
	setInbound(&options, &opt)
	setDns(&options, &opt)
	// fmt.Printf("[debug] Region=%q BlockAds=%v EnableDNSRouting=%v ConnTestUrl=%q\n",
	// opt.Region, opt.BlockAds, opt.EnableDNSRouting, opt.ConnectionTestUrl)
	setRoutingOptions(&options, &opt)
	setFakeDns(&options, &opt)
	err := setOutbounds(&options, &input, &opt)
	if err != nil {
		return nil, err
	}

	return &options, nil
}

func addForceDirect(options *option.Options, opt *RostovVPNOptions, directDNSDomains map[string]bool) {
	if options.DNS == nil || len(directDNSDomains) == 0 {
		return
	}
	directDomains := make([]string, 0, len(directDNSDomains))
	// fmt.Println("[addForceDirect] !!! \n", directDomains, "\n !!! [addForceDirect]")

	for domain := range directDNSDomains {
		if domain == "" {
			continue
		}
		directDomains = append(directDomains, domain)
	}
	if len(directDomains) == 0 {
		return
	}
	sort.Strings(directDomains)

	// На Android/TUN резолв доменов серверов — через UDP bootstrap, чтобы избежать
	// зависимости от HTTPS-детура на старте. На прочих ОС оставляем trick-DoH.
	serverTag := DNSTricksDirectTag
	if runtime.GOOS == "android" && (opt != nil && (opt.EnableTun || opt.EnableTunService)) {
		serverTag = DNSBootstrapTag
	}

	dnsRule := option.DefaultDNSRule{
		RawDefaultDNSRule: option.RawDefaultDNSRule{
			Domain: directDomains, // <— массив доменов (например, ["rostovvpn.run.place"])
		},
		DNSRuleAction: option.DNSRuleAction{
			Action: C.RuleActionTypeRoute,
			RouteOptions: option.DNSRouteActionOptions{
				Server: serverTag,
			},
		},
	}

	// Препендим, чтобы оно сработало раньше общих правил
	options.DNS.Rules = append([]option.DNSRule{{Type: C.RuleTypeDefault, DefaultOptions: dnsRule}}, options.DNS.Rules...)
}
func setOutbounds(options *option.Options, input *option.Options, opt *RostovVPNOptions) error {
	directDNSDomains := make(map[string]bool)
	staticIPs := make(map[string][]string)
	var outbounds []option.Outbound
	var tags []string
	OutboundMainProxyTag = OutboundSelectTag

	// --- главный цикл по входным аутбаундам БЕЗ map/struct-раунда ---
	for _, base := range input.Outbounds {
		upd, serverDomain, err := patchOutboundSafe(base, *opt, staticIPs)
		if err != nil || upd == nil {
			upd = &base
		}
		// Страховка: не допускаем Options=nil у протокольных аутбаундов
		if upd.Options == nil && base.Options != nil {
			upd.Options = base.Options
		}

		if serverDomain != "" {
			directDNSDomains[serverDomain] = true
		}

		switch strings.ToLower(upd.Type) {
		case strings.ToLower(C.TypeDirect),
			strings.ToLower(C.TypeBlock),
			strings.ToLower(C.TypeDNS),
			strings.ToLower(C.TypeSelector),
			strings.ToLower(C.TypeURLTest):
			continue
		}

		if upd.Tag == "" {
			upd.Tag = fmt.Sprintf("outbound-%d", len(tags))
		}
		if !strings.Contains(strings.ToLower(upd.Tag), "hide") {
			tags = append(tags, upd.Tag)
		}

		// ВАЖНО: НЕ делаем mapPatch — сохраняем типизированные Options
		outbounds = append(outbounds, *upd)
	}

	urlTest := option.Outbound{
		Type: C.TypeURLTest,
		Tag:  OutboundURLTestTag,
		Options: &option.URLTestOutboundOptions{
			Outbounds:                 tags,
			URL:                       opt.ConnectionTestUrl,
			Interval:                  badoption.Duration(opt.URLTestInterval.Duration()),
			Tolerance:                 1,
			IdleTimeout:               badoption.Duration(opt.URLTestInterval.Duration() * 3),
			InterruptExistConnections: true,
		},
	}

	url := opt.ConnectionTestUrl
	if strings.HasPrefix(url, "http://cp.cloudflare.com") || url == "" {
		url = "http://1.1.1.1/cdn-cgi/trace" // чистый IP — без DNS
	}
	urlTest.Options.(*option.URLTestOutboundOptions).URL = url

	defaultSelect := urlTest.Tag
	if len(tags) > 0 {
		defaultSelect = tags[0]
	}
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), "default") {
			defaultSelect = tag
			break
		}
	}
	selector := option.Outbound{
		Type: C.TypeSelector,
		Tag:  OutboundSelectTag,
		Options: &option.SelectorOutboundOptions{
			Outbounds:                 append([]string{urlTest.Tag}, tags...),
			Default:                   defaultSelect,
			InterruptExistConnections: true,
		},
	}

	outbounds = append([]option.Outbound{selector, urlTest}, outbounds...)

	// Базовые аутбаунды — без несуществующих Options (nil это нормально)
	baseOutbounds := []option.Outbound{
		// DNS/Block — «заглушечные» опции StubOptions
		{Type: C.TypeDNS, Tag: OutboundDNSTag, Options: &option.StubOptions{}},
		{Type: C.TypeDirect, Tag: OutboundDirectTag, Options: &option.DirectOutboundOptions{}},
		{Type: C.TypeDirect, Tag: OutboundBypassTag, Options: &option.DirectOutboundOptions{}},
		{Type: C.TypeBlock, Tag: OutboundBlockTag, Options: &option.StubOptions{}},
	}
	options.Outbounds = append(outbounds, baseOutbounds...)

	addForceDirect(options, opt, directDNSDomains)
	applyStaticIPHosts(options, staticIPs)
	return nil
}
func patchOutboundSafe(base option.Outbound, opt RostovVPNOptions, staticIPs map[string][]string) (*option.Outbound, string, error) {
	o := base
	var serverDomain string

	switch strings.ToLower(base.Type) {
	case strings.ToLower(C.TypeVLESS):
		if v, ok := base.Options.(*option.VLESSOutboundOptions); ok && v != nil {
			if v.Server != "" && net.ParseIP(v.Server) == nil {
				serverDomain = v.Server
			}

			if v.TLS != nil && v.TLS.ServerName == "www.github.com" {
				v.TLS.ServerName = "github.com"
			}
			// тут при желании можно что-то чуть-чуть «доподкрутить», не меняя типы
			// например:
			// if v.TLS != nil && v.TLS.UTLS != nil && v.TLS.UTLS.Enabled && v.TLS.UTLS.Fingerprint == "" {
			//     v.TLS.UTLS.Fingerprint = "random"
			// }
			// 3) вернуть изменённые options (на случай копирования интерфейса)
			o.Options = v
		}
		// при необходимости добавите vmess/trojan/hysteria и т.д. по аналогии
	}

	return &o, serverDomain, nil
}
func setClashAPI(options *option.Options, opt *RostovVPNOptions) {
	if opt.EnableClashApi {
		if opt.ClashApiSecret == "" {
			opt.ClashApiSecret = generateRandomString(16)
		}
		options.Experimental = &option.ExperimentalOptions{
			ClashAPI: &option.ClashAPIOptions{
				ExternalController: fmt.Sprintf("%s:%d", "127.0.0.1", opt.ClashApiPort),
				Secret:             opt.ClashApiSecret,
			},

			CacheFile: &option.CacheFileOptions{
				Enabled: true,
				Path:    "clash.db",
			},
		}
	}
}

func setLog(options *option.Options, opt *RostovVPNOptions) {
	output := opt.LogFile
	if output == "" {
		output = "box.log" // будет создан рядом с last_config.json
	}
	options.Log = &option.LogOptions{
		Level:        opt.LogLevel,
		Output:       output,
		Disabled:     false,
		Timestamp:    true,
		DisableColor: true,
	}
}

func setInbound(options *option.Options, opt *RostovVPNOptions) {
	var inboundDomainStrategy option.DomainStrategy
	if !opt.ResolveDestination {
		inboundDomainStrategy = option.DomainStrategy(dns.DomainStrategyAsIS)
	} else {
		inboundDomainStrategy = opt.IPv6Mode
	}

	// TUN поднимаем только при EnableTun
	if opt.EnableTun {
		if opt.MTU == 0 || opt.MTU > 2000 {
			opt.MTU = 1450
		}

		// ВАЖНО:
		// 1) Всегда массив (даже при IPv4-only), иначе Listable сериализуется в строку.
		// 2) Используем /30 (IPv4) и /126 (IPv6) для p2p — это стабильно на Android/gVisor.
		addressList := badoption.Listable[netip.Prefix]{
			netip.MustParsePrefix("172.19.0.1/30"),
			netip.MustParsePrefix("fdfe:dcba:9876::1/126"),
		}

		tunOptions := &option.TunInboundOptions{
			Stack:         opt.TUNStack,
			MTU:           opt.MTU,
			AutoRoute:     true,
			StrictRoute:   opt.StrictRoute,
			Address:       addressList, // ← массив, не скаляр
			InterfaceName: "RostovVPNTunnel",
			InboundOptions: option.InboundOptions{
				SniffEnabled:             true,
				SniffOverrideDestination: false,
				DomainStrategy:           inboundDomainStrategy,
			},
		}

		// Android-специфика: не захватываем трафик своего приложения
		// и не «бетонируем» маршруты — прямые сокеты смогут найти маршрут.
		if runtime.GOOS == "android" {
			// tunOptions.StrictRoute = false
			tunOptions.ExcludePackage = badoption.Listable[string]{"app.rostovvpn.com"}
		}

		options.Inbounds = append(options.Inbounds, option.Inbound{
			Type:    C.TypeTun,
			Tag:     InboundTUNTag,
			Options: tunOptions,
		})
	}

	bind := "127.0.0.1"
	if opt.AllowConnectionFromLAN {
		bind = "0.0.0.0"
	}

	mixedOptions := &option.HTTPMixedInboundOptions{
		ListenOptions: option.ListenOptions{
			Listen:     addrPtr(bind),
			ListenPort: opt.MixedPort,
			InboundOptions: option.InboundOptions{
				SniffEnabled:             true,
				SniffOverrideDestination: false, // был true
				DomainStrategy:           inboundDomainStrategy,
			},
		},
		SetSystemProxy: opt.SetSystemProxy,
	}
	options.Inbounds = append(options.Inbounds, option.Inbound{
		Type:    C.TypeMixed,
		Tag:     InboundMixedTag,
		Options: mixedOptions,
	})

	directDNSOptions := &option.DirectInboundOptions{
		ListenOptions: option.ListenOptions{
			Listen:     addrPtr(bind),
			ListenPort: opt.LocalDnsPort,
		},
	}
	options.Inbounds = append(options.Inbounds, option.Inbound{
		Type:    C.TypeDirect,
		Tag:     InboundDNSTag,
		Options: directDNSOptions,
	})

	if opt.EnableTunService {
		ActivateTunnelService(*opt)
	}
}

func setDns(options *option.Options, opt *RostovVPNOptions) {
	dnsOptions := &option.DNSOptions{}
	dnsOptions.Final = DNSRemoteTag
	dnsOptions.DNSClientOptions = option.DNSClientOptions{
		IndependentCache: opt.IndependentDNSCache,
	}
	// ВСЕГДА держим и прокси-DNS, и бутстрап-DoH, и local.
	// dnsOptions.Servers = []option.DNSServerOptions{
	// 	// 1) основной резолвер через прокси (для всего остального трафика)
	// 	newDNSServer(DNSRemoteTag, normalizeDNSAddress(opt.RemoteDnsAddress), DNSLocalTag, opt.RemoteDnsDomainStrategy, OutboundSelectTag),
	// 	// 2) бутстрап-DoH, который ходит напрямую (anti-DPI)
	// 	// ВАЖНО: DoH сам себя резолвит через hosts (dns-warp-hosts), detour не задаём
	// 	newDNSServer(DNSTricksDirectTag, "https://sky.rethinkdns.com/", DNSWarpHostsTag, opt.DirectDnsDomainStrategy, ""),
	// 	// 3) опциональный plain UDP DNS напрямую (пусть будет как запасной)
	// 	newDNSServer(DNSDirectTag, normalizeDNSAddress(opt.DirectDnsAddress), DNSLocalTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),
	// 	// 4) local/system
	// 	newDNSServer(DNSLocalTag, "local", "", 0, ""),
	// }
	// --- BOOTSTRAP DNS (всегда DIRECT) ---
	bootstrap := normalizeDNSAddress(opt.DirectDnsAddress)
	if strings.TrimSpace(bootstrap) == "" {
		bootstrap = "udp://8.8.8.8:53"
	}
	// --- DNS-REMOTE (DoH) ---
	// имя DoH-хоста (cloudflare-dns.com) резолвим через "трик‑DoH" (sky.rethinkdns.com)
	remoteResolver := DNSTricksDirectTag
	// ANDROID/TUN: чтобы избежать петли старта (DoH→select, а select ещё не готов),
	// на Android/TUN отправляем сам DoH напрямую (direct).
	dnsRemoteDetour := OutboundSelectTag
	if runtime.GOOS == "android" && (opt.EnableTun || opt.EnableTunService) {
		dnsRemoteDetour = OutboundDirectTag
	}

	dnsOptions.Servers = []option.DNSServerOptions{
		// 0) bootstrap UDP → direct (всегда)
		newDNSServer(DNSBootstrapTag, bootstrap, DNSLocalTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),
		// 1) DoH Cloudflare; ANDROID/TUN: detour=direct (см. выше)
		newDNSServer(DNSRemoteTag, pickProxyDNS(opt.RemoteDnsAddress, runtime.GOOS == "android" || opt.EnableTun || opt.EnableTunService), remoteResolver, opt.RemoteDnsDomainStrategy, dnsRemoteDetour),
		// 2) trick‑DoH RethinkDNS → direct, с хостами
		newDNSServer(DNSTricksDirectTag, "https://sky.rethinkdns.com/", DNSWarpHostsTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),
		// 3) local/system
		newDNSServer(DNSLocalTag, "local", "", 0, ""),
	}
	options.DNS = dnsOptions
	// fmt.Println("[setDns] !!! \n", dnsOptions, "\n !!! [setDns]")

	// // ВАЖНО: если включены TLS-tricks — прокинем их прямо в DoH (dns-trick-direct)
	// injectTLSTricksIntoDoH(options, opt)
	// ВНИМАНИЕ: upstream xtls/sing-box не поддерживает tls_fragment/tls_padding/mixed_sni_case
	// у DNS-серверов. Чтобы не уронить парсер — ничего не инжектим, но выводим предупреждение,
	// если флаги включены.
	warnIfTLSTricksRequestedButUnsupported(opt)

	// if ips := getIPs([]string{"www.speedtest.net", "sky.rethinkdns.com"}); len(ips) > 0 {
	// 	applyStaticIPHosts(options, map[string][]string{"sky.rethinkdns.com": ips})
	// }
	// Гарантируем IP для sky.rethinkdns.com даже если системный DNS мёртв на старте.
	{
		applyStaticIPHosts(options, map[string][]string{
			"sky.rethinkdns.com": {"104.17.147.22", "104.17.148.22", "104.18.0.48", "104.18.1.48"},
		})
	}
	{
		applyStaticIPHosts(options, map[string][]string{"cloudflare-dns.com": []string{"1.1.1.1", "1.0.0.1", "1.1.1.2", "1.0.0.2"}})
	}
}
func setFakeDns(options *option.Options, opt *RostovVPNOptions) {
	if options.DNS == nil || !opt.EnableFakeDNS {
		return
	}

	inet4Range := badoption.Prefix(netip.MustParsePrefix("198.18.0.0/15"))
	inet6Range := badoption.Prefix(netip.MustParsePrefix("fc00::/18"))
	options.DNS.FakeIP = &option.LegacyDNSFakeIPOptions{
		Enabled:    true,
		Inet4Range: &inet4Range,
		Inet6Range: &inet6Range,
	}

	options.DNS.Servers = append(options.DNS.Servers, legacyDNSServer(DNSFakeTag, "fakeip", "", option.DomainStrategy(dns.DomainStrategyUseIPv4), ""))

	dnsRule := option.DefaultDNSRule{
		RawDefaultDNSRule: option.RawDefaultDNSRule{
			Inbound: []string{InboundTUNTag},
		},
		DNSRuleAction: option.DNSRuleAction{
			Action: C.RuleActionTypeRoute,
			RouteOptions: option.DNSRouteActionOptions{
				Server:       DNSFakeTag,
				DisableCache: true,
			},
		},
	}
	// fmt.Println("[setFakeDns] !!! \n", dnsRule, "\n !!! [setFakeDns]")

	options.DNS.Rules = append(options.DNS.Rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: dnsRule})
}
func setRoutingOptions(options *option.Options, opt *RostovVPNOptions) {
	if options.DNS == nil {
		options.DNS = &option.DNSOptions{}
	}

	dnsRules := make([]option.DNSRule, 0)
	routeRules := make([]option.Rule, 0)
	rulesets := make([]option.RuleSet, 0)

	routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{Inbound: []string{InboundDNSTag}}, OutboundDNSTag))
	routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{Port: []uint16{53}}, OutboundDNSTag))
	// routeRules = append(routeRules, newRouteRule(
	// 	option.RawDefaultRule{Domain: []string{"api.ip.sb", "ipapi.co", "ipinfo.io"}},
	// 	OutboundDirectTag,
	// ))
	// ANDROID: Разрешаем DoT (853) к приватным IP (в т.ч. 172.19.0.2 — peer TUN DNS),
	// чтобы системный Private DNS не ломал старт, а всё остальное по 853 блокируем.
	if runtime.GOOS == "android" {
		// 1) allow: 853 к приватным адресам (RFC1918, сюда попадает 172.19.0.2)
		routeRules = append(routeRules, newRouteRule(
			option.RawDefaultRule{
				IPIsPrivate: true,
				Port:        []uint16{853},
			},
			OutboundDirectTag,
		))
		// 2) block: все остальные 853
		routeRules = append(routeRules, newRouteRule(
			option.RawDefaultRule{Port: []uint16{853}},
			OutboundBlockTag,
		))
	}

	if opt.BypassLAN {
		routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{IPIsPrivate: true}, OutboundBypassTag))
	}

	// В режиме TUN-сервиса не уводим ничего в direct.
	// ВСЕГДА: трафик к DoH-хосту идёт напрямую, чтобы бутстрап не зависел от прокси
	routeRules = append(routeRules, newRouteRule(
		option.RawDefaultRule{Domain: []string{"sky.rethinkdns.com"}},
		OutboundDirectTag,
	))

	// В TunService НЕ гоним cloudflare-dns.com в direct:
	// пусть DoH может идти через прокси. Иначе при блокировке CF DoH ломается весь DNS.
	if !opt.EnableTunService {
		routeRules = append(routeRules, newRouteRule(
			option.RawDefaultRule{Domain: []string{"cloudflare-dns.com"}},
			OutboundDirectTag,
		))
	}
	// DNS для самого DoH-хоста отдаём бутстрапу/hosts (applyStaticIPHosts уже сделал правило dns-warp-hosts),
	// отдельное DNS-правило здесь не нужно. Если хочешь принудительно — можно так:
	// dnsRules = append(dnsRules, newDNSRouteRule(
	//     option.DefaultDNSRule{RawDefaultDNSRule: option.RawDefaultDNSRule{Domain: []string{"sky.rethinkdns.com"}}},
	//     DNSWarpHostsTag,
	// ))

	// fmt.Println("[setRoutingOptions] !!! dnsRules=\n", dnsRules, "]n !!! [setRoutingOptions]")

	for _, rule := range opt.Rules {
		routeRule := rule.MakeRule()
		var outbound string
		switch rule.Outbound {
		case "bypass":
			outbound = OutboundBypassTag
		case "block":
			outbound = OutboundBlockTag
		case "proxy":
			outbound = OutboundMainProxyTag
		}
		if outbound != "" {
			routeRule.RuleAction = option.RuleAction{
				Action:       C.RuleActionTypeRoute,
				RouteOptions: option.RouteActionOptions{Outbound: outbound},
			}
			if routeRule.IsValid() {
				routeRules = append(routeRules, option.Rule{Type: C.RuleTypeDefault, DefaultOptions: routeRule})
			}
		}

		dnsRule := rule.MakeDNSRule()
		routeOpts := dnsRule.DNSRuleAction.RouteOptions
		var server string
		switch rule.Outbound {
		case "bypass":
			// server = DNSDirectTag
			server = DNSBootstrapTag
		case "block":
			{
				rc := option.DNSRCode(0) // или "REFUSED"/"NXDOMAIN" — как хочешь блокировать
				dnsRule.DNSRuleAction = option.DNSRuleAction{
					Action: C.RuleActionTypePredefined,
					PredefinedOptions: option.DNSRouteActionPredefined{
						Rcode: &rc,
					},
				}
				dnsRules = append(dnsRules, option.DNSRule{
					Type:           C.RuleTypeDefault,
					DefaultOptions: dnsRule,
				})
			}
			continue // важно: чтобы ниже не перезаписать dnsRule через RouteOptions
		case "proxy":
			server = DNSRemoteTag
			if opt.EnableFakeDNS {
				fakeRule := dnsRule
				fakeRule.Inbound = []string{InboundTUNTag, InboundMixedTag}
				fakeRule.DNSRuleAction = option.DNSRuleAction{
					Action: C.RuleActionTypeRoute,
					RouteOptions: option.DNSRouteActionOptions{
						Server:       DNSFakeTag,
						DisableCache: true,
					},
				}
				dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: fakeRule})
			}
		}
		if server != "" {
			routeOpts.Server = server
			dnsRule.DNSRuleAction = option.DNSRuleAction{
				Action:       C.RuleActionTypeRoute,
				RouteOptions: routeOpts,
			}
			dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: dnsRule})
		}
	}

	if parsedURL, err := url.Parse(opt.ConnectionTestUrl); err == nil {
		var ttl uint32 = 3000
		dnsRule := option.DefaultDNSRule{
			RawDefaultDNSRule: option.RawDefaultDNSRule{Domain: []string{parsedURL.Host}},
			DNSRuleAction: option.DNSRuleAction{
				Action: C.RuleActionTypeRoute,
				RouteOptions: option.DNSRouteActionOptions{
					Server:     DNSRemoteTag,
					RewriteTTL: &ttl,
				},
			},
		}
		dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: dnsRule})
	}
	// fmt.Println("[setRoutingOptions] !!! \n", *opt, "]n !!! [setRoutingOptions]")
	if opt.BlockAds {
		blockRuleSets := []struct {
			Tag string
			URL string
		}{
			{"geosite-ads", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geosite-category-ads-all.srs"},
			{"geosite-malware", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geosite-malware.srs"},
			{"geosite-phishing", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geosite-phishing.srs"},
			{"geosite-cryptominers", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geosite-cryptominers.srs"},
			{"geoip-phishing", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geoip-phishing.srs"},
			{"geoip-malware", "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/block/geoip-malware.srs"},
		}
		ruleSetTags := make([]string, 0, len(blockRuleSets))
		for _, rs := range blockRuleSets {
			ruleSetTags = append(ruleSetTags, rs.Tag)
			rulesets = append(rulesets, newRemoteRuleSet(rs.Tag, rs.URL))
		}
		routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{RuleSet: ruleSetTags}, OutboundBlockTag))
		// dnsRules = append(dnsRules,
		// 	newDNSPredefinedRule(
		// 		option.DefaultDNSRule{
		// 			RawDefaultDNSRule: option.RawDefaultDNSRule{
		// 				RuleSet: ruleSetTags,
		// 			},
		// 		},
		// 		option.DNSRCode(3),
		// 	),
		// )
	}

	if opt.Region != "other" {
		regionRuleSets := []struct {
			Tag string
			URL string
		}{
			{"geoip-" + opt.Region, "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/country/geoip-" + opt.Region + ".srs"},
			{"geosite-" + opt.Region, "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/country/geosite-" + opt.Region + ".srs"},
		}
		regionTags := make([]string, 0, len(regionRuleSets))
		for _, rs := range regionRuleSets {
			regionTags = append(regionTags, rs.Tag)
			rulesets = append(rulesets, newRemoteRuleSet(rs.Tag, rs.URL))
		}
		// dnsRules = append(dnsRules, newDNSRouteRule(
		// 	option.DefaultDNSRule{
		// 		RawDefaultDNSRule: option.RawDefaultDNSRule{
		// 			DomainSuffix: []string{"." + opt.Region},
		// 		},
		// 	},
		// 	DNSDirectTag,
		// ))
		// fmt.Println("[setRoutingOptions] not other!!! \n", opt.Region, "\n\n\n\n\n", dnsRules, "]n !!! [setRoutingOptions]")

		// routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{RuleSet: regionTags}, OutboundDirectTag))
		// dnsRules = append(dnsRules, newDNSRouteRule(option.DefaultDNSRule{RawDefaultDNSRule: option.RawDefaultDNSRule{RuleSet: regionTags}}, DNSDirectTag))
		if opt.EnableTunService || runtime.GOOS == "android" {
			// 1) По умолчанию (без флагов) оставляем как есть: DNS для RU через прокси-DoH
			dnsRules = append(dnsRules, newDNSRouteRule(
				option.DefaultDNSRule{
					RawDefaultDNSRule: option.RawDefaultDNSRule{
						RuleSet: regionTags,
					},
				},
				DNSRemoteTag,
			))

			routeRules = append(routeRules, newRouteRule(
				option.RawDefaultRule{RuleSet: regionTags},
				OutboundDirectTag,
			))
		} else {
			// Старое поведение вне TUN
			dnsRules = append(dnsRules, newDNSRouteRule(
				option.DefaultDNSRule{
					RawDefaultDNSRule: option.RawDefaultDNSRule{
						DomainSuffix: []string{"." + opt.Region},
					},
				},
				DNSBootstrapTag,
			))
			// fmt.Println("[setRoutingOptions] not other!!! \n", opt.Region, "\n\n\n\n\n", dnsRules, "]n !!! [setRoutingOptions]")

			routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{RuleSet: regionTags}, OutboundDirectTag))
			dnsRules = append(dnsRules, newDNSRouteRule(
				option.DefaultDNSRule{RawDefaultDNSRule: option.RawDefaultDNSRule{RuleSet: regionTags}},
				DNSBootstrapTag,
			))
		}
	}
	// ANDROID/TUN: авто‑детект интерфейса может давать "no available network interface".
	// Выключаем его на Android/TUN.
	shouldAutoDetect := !(runtime.GOOS == "android" && (opt.EnableTun || opt.EnableTunService))
	if options.Route == nil {
		options.Route = &option.RouteOptions{
			Final:               OutboundMainProxyTag,
			AutoDetectInterface: shouldAutoDetect,
			OverrideAndroidVPN:  runtime.GOOS == "android" && opt.PerAppProxyMode == "off",
			DefaultDomainResolver: &option.DomainResolveOptions{
				Server: pickDefaultResolver(opt),
			},
		}
	} else {
		if options.Route.Final == "" {
			options.Route.Final = OutboundMainProxyTag
		}
		options.Route.AutoDetectInterface = shouldAutoDetect
		options.Route.OverrideAndroidVPN = runtime.GOOS == "android" && opt.PerAppProxyMode == "off"
		if options.Route.DefaultDomainResolver == nil {
			options.Route.DefaultDomainResolver = &option.DomainResolveOptions{Server: pickDefaultResolver(opt)}
		} else if options.Route.DefaultDomainResolver.Server == "" {
			options.Route.DefaultDomainResolver.Server = pickDefaultResolver(opt)
		}
	}

	// Не задаём DefaultNetworkStrategy, если пользователь не попросил.
	// Если пользователь явно указал стратегию — она важнее всего.
	// Жёстко форсим IPv4, если пользователь явно не задал иную стратегию
	if opt.DefaultNetworkStrategy == "" && opt.IPv6Mode == option.DomainStrategy(dns.DomainStrategyUseIPv4) {
		if ns := toNetworkStrategyPtr("force_ipv4"); ns != nil {
			options.Route.DefaultNetworkStrategy = ns
		}
	}
	if ns := toNetworkStrategyPtr(opt.DefaultNetworkStrategy); ns != nil {
		options.Route.DefaultNetworkStrategy = ns
	} else if options.Route.DefaultNetworkStrategy == nil {
		// <<< ВСТАВКА: автоматическое наследование из IPv6Mode >>>
		// Если явная стратегия сети не задана — синхронизируем с IPv6Mode.
		switch opt.IPv6Mode {
		case option.DomainStrategy(dns.DomainStrategyUseIPv4):
			if ns := toNetworkStrategyPtr("force_ipv4"); ns != nil {
				options.Route.DefaultNetworkStrategy = ns
			}
		case option.DomainStrategy(dns.DomainStrategyUseIPv6):
			if ns := toNetworkStrategyPtr("force_ipv6"); ns != nil {
				options.Route.DefaultNetworkStrategy = ns
			}
		case option.DomainStrategy(dns.DomainStrategyPreferIPv4):
			if ns := toNetworkStrategyPtr("prefer_ipv4"); ns != nil {
				options.Route.DefaultNetworkStrategy = ns
			}
		case option.DomainStrategy(dns.DomainStrategyPreferIPv6):
			if ns := toNetworkStrategyPtr("prefer_ipv6"); ns != nil {
				options.Route.DefaultNetworkStrategy = ns
			}
		default:
			if ns := toNetworkStrategyPtr("force_ipv4"); ns != nil {
				options.Route.DefaultNetworkStrategy = ns
			}
		}
		// "prefer_ipv4", "prefer_ipv6", "force_ipv4", "force_ipv6".
		// >>> Конец вставки
	}
	// fmt.Println("[setRoutingOptions] !!! Rules\n", routeRules, "\n !!! [setRoutingOptions]")

	options.Route.Rules = append(options.Route.Rules, routeRules...)
	options.Route.RuleSet = append(options.Route.RuleSet, rulesets...)

	if opt.EnableDNSRouting {
		// fmt.Println("[setRoutingOptions] !!! dnsRules\n", dnsRules, "\n !!! [setRoutingOptions]")

		options.DNS.Rules = append(options.DNS.Rules, dnsRules...)
	}
}

// ПРИМЕЧАНИЕ: проверь точные токены в твоей версии sing-box.
// На новых версиях обычно такие:
// "prefer_ipv4", "prefer_ipv6", "force_ipv4", "force_ipv6".
func toNetworkStrategyPtr(s string) *option.NetworkStrategy {
	if s == "" {
		return nil
	}
	if ns, ok := C.StringToNetworkStrategy[s]; ok {
		v := option.NetworkStrategy(ns)
		return &v
	}
	return nil
}

func pickDefaultResolver(opt *RostovVPNOptions) string {
	if opt.DefaultDomainResolver != "" {
		return opt.DefaultDomainResolver
	}
	// На Android/TUN используем bootstrap (udp 8.8.8.8) как дефолтный резолвер
	// для внутренних резолвов (включая резолв аутбаундов), чтобы исключить
	// цикл «dns-remote → select → vless(нужен DNS)» на старте.
	if opt != nil && (runtime.GOOS == "android" && (opt.EnableTun || opt.EnableTunService)) {
		return DNSBootstrapTag
	}
	// На прочих ОС оставляем прежнее безопасное значение.
	return DNSTricksDirectTag
}
func legacyDNSServer(tag, address, resolver string, strategy option.DomainStrategy, detour string) option.DNSServerOptions {
	legacy := &option.LegacyDNSServerOptions{
		Address: address,
	}
	if resolver != "" {
		legacy.AddressResolver = resolver
	}
	if strategy != 0 {
		legacy.Strategy = strategy
	}
	if detour != "" {
		legacy.Detour = detour
	}
	return option.DNSServerOptions{
		Type:    C.DNSTypeLegacy,
		Tag:     tag,
		Options: legacy,
	}
}

func newRemoteRuleSet(tag, url string) option.RuleSet {
	remoteRuleset := option.RuleSet{
		Type:   C.RuleSetTypeRemote,
		Tag:    tag,
		Format: C.RuleSetFormatBinary,
		RemoteOptions: option.RemoteRuleSet{
			URL:            url,
			UpdateInterval: badoption.Duration(5 * 24 * time.Hour),
			DownloadDetour: OutboundSelectTag,
		},
	}
	if runtime.GOOS == "android" {
		remoteRuleset.RemoteOptions.DownloadDetour = OutboundDirectTag
	}
	return remoteRuleset

}

func newRouteRule(match option.RawDefaultRule, outbound string) option.Rule {
	return option.Rule{
		Type: C.RuleTypeDefault,
		DefaultOptions: option.DefaultRule{
			RawDefaultRule: match,
			RuleAction: option.RuleAction{
				Action:       C.RuleActionTypeRoute,
				RouteOptions: option.RouteActionOptions{Outbound: outbound},
			},
		},
	}
}

func newDNSRouteRule(rule option.DefaultDNSRule, server string) option.DNSRule {
	options := rule.DNSRuleAction.RouteOptions
	options.Server = server
	rule.DNSRuleAction = option.DNSRuleAction{
		Action:       C.RuleActionTypeRoute,
		RouteOptions: options,
	}
	return option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: rule}
}

func newDNSPredefinedRule(rule option.DefaultDNSRule, rcode option.DNSRCode) option.DNSRule {
	rule.DNSRuleAction = option.DNSRuleAction{
		Action: C.RuleActionTypePredefined,
		PredefinedOptions: option.DNSRouteActionPredefined{
			Rcode: &rcode, // например: "SUCCESS", "REFUSED", "NXDOMAIN"
		},
	}
	return option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: rule}
}

// ----- TLS tricks injection for DoH (dns-trick-direct) -----
func hasTLSTricks(opt *RostovVPNOptions) bool {
	if opt == nil {
		return false
	}
	// считаем включённым, если активен любой из флагов
	return opt.TLSTricks.EnableFragment || opt.TLSTricks.MixedSNICase || opt.TLSTricks.EnablePadding
}

// По умолчанию — только варнинг. Инъекцию не делаем, чтобы не сломать парсер.
func warnIfTLSTricksRequestedButUnsupported(opt *RostovVPNOptions) {
	if hasTLSTricks(opt) {
		fmt.Println("[dns] TLS tricks (fragment/padding/mixed_sni_case) запрошены," +
			" но текущая сборка sing-box (xtls upstream) их не поддерживает на DNS-серверах; пропускаю.")
	}
}

func applyStaticIPHosts(options *option.Options, records map[string][]string) {
	if options.DNS == nil || len(records) == 0 {
		return
	}

	normalized := make(map[string][]netip.Addr)
	domains := make([]string, 0)
	for domain, ips := range records {
		var addrs []netip.Addr
		for _, ip := range ips {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				continue
			}
			addrs = append(addrs, addr)
		}
		if len(addrs) == 0 {
			continue
		}
		lower := strings.ToLower(domain)
		normalized[lower] = addrs
		domains = append(domains, lower)
	}
	if len(normalized) == 0 {
		return
	}
	sort.Strings(domains)

	updated := false
	for i := range options.DNS.Servers {
		if options.DNS.Servers[i].Tag != DNSWarpHostsTag {
			continue
		}
		if hosts, ok := options.DNS.Servers[i].Options.(*option.HostsDNSServerOptions); ok {
			if hosts.Predefined == nil {
				hosts.Predefined = &badjson.TypedMap[string, badoption.Listable[netip.Addr]]{}
			}
			for domain, addrs := range normalized {
				list := make(badoption.Listable[netip.Addr], 0, len(addrs))
				for _, addr := range addrs {
					list = append(list, addr)
				}
				hosts.Predefined.Put(domain, list)
			}
			options.DNS.Servers[i].Options = hosts
			updated = true
			break
		}
	}

	if !updated {
		hosts := &option.HostsDNSServerOptions{Predefined: &badjson.TypedMap[string, badoption.Listable[netip.Addr]]{}}
		for domain, addrs := range normalized {
			list := make(badoption.Listable[netip.Addr], 0, len(addrs))
			for _, addr := range addrs {
				list = append(list, addr)
			}
			hosts.Predefined.Put(domain, list)
		}
		options.DNS.Servers = append(options.DNS.Servers, option.DNSServerOptions{
			Type:    C.DNSTypeHosts,
			Tag:     DNSWarpHostsTag,
			Options: hosts,
		})
	}

	dnsRule := option.DefaultDNSRule{
		RawDefaultDNSRule: option.RawDefaultDNSRule{
			Domain: domains,
		},
		DNSRuleAction: option.DNSRuleAction{
			Action: C.RuleActionTypeRoute,
			RouteOptions: option.DNSRouteActionOptions{
				Server:       DNSWarpHostsTag,
				DisableCache: true,
			},
		},
	}
	// fmt.Println("[applyStaticIPHosts] !!! dnsRule\n", dnsRule, "\n !!! [applyStaticIPHosts]")

	options.DNS.Rules = append(options.DNS.Rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: dnsRule})
}

func addrPtr(value string) *badoption.Addr {
	if value == "" {
		return nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return nil
	}
	converted := badoption.Addr(addr)
	return &converted
}

func patchRostovVPNWarpFromConfigMap(obj outboundMap, opt RostovVPNOptions) outboundMap {
	if opt.Warp.EnableWarp && opt.Warp.Mode == "proxy_over_warp" {
		if obj.string("detour") == "" {
			obj["detour"] = "rostovvpn-warp"
		}
	}
	return obj
}

func getIPs(domains []string) []string {
	res := []string{}
	for _, d := range domains {
		ips, err := net.LookupHost(d)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if !strings.HasPrefix(ip, "10.") {
				res = append(res, ip)
			}
		}
	}
	return res
}

func isBlockedDomain(domain string) bool {
	if strings.HasPrefix("full:", domain) {
		return false
	}
	ips, err := net.LookupHost(domain)
	if err != nil {
		// fmt.Println(err)
		return true
	}

	// Print the IP addresses associated with the domain
	fmt.Printf("IP addresses for %s:\n", domain)
	for _, ip := range ips {
		if strings.HasPrefix(ip, "10.") {
			return true
		}
	}
	return false
}

func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

func generateRandomString(length int) string {
	// Determine the number of bytes needed
	bytesNeeded := (length*6 + 7) / 8

	// Generate random bytes
	randomBytes := make([]byte, bytesNeeded)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "rostovvpn"
	}

	// Encode random bytes to base64
	randomString := base64.URLEncoding.EncodeToString(randomBytes)

	// Trim padding characters and return the string
	return randomString[:length]
}

// helper: если включён TUN-service и адрес udp:// — использовать DoT
func pickProxyDNS(addr string, tunService bool) string {
	a := normalizeDNSAddress(addr)
	low := strings.ToLower(a)
	if tunService {
		// fmt.Println("[pickProxyDNS] !!! tunService\n", tunService, "\na=\n", a, "\n", a == "" || strings.HasPrefix(low, "udp://") || strings.HasPrefix(low, "tls://"), "\n !!! [pickProxyDNS]")

		if a == "" || strings.HasPrefix(low, "udp://") || strings.HasPrefix(low, "tls://") {
			// Prefer DoH in TUN-service mode: Cloudflare endpoint works reliably over HTTPS
			return "https://cloudflare-dns.com/dns-query"
		}
	}
	return a
}

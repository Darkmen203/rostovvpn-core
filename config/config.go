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
	badjson "github.com/sagernet/sing/common/json/badjson"
	badoption "github.com/sagernet/sing/common/json/badoption"
)

const (
	DNSRemoteTag       = "dns-remote"
	DNSLocalTag        = "dns-local"
	DNSDirectTag       = "dns-direct"
	DNSBlockTag        = "dns-block"
	DNSFakeTag         = "dns-fake"
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
	var buffer bytes.Buffer
	json.NewEncoder(&buffer)
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(options)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// TODO include selectors
func BuildConfig(opt RostovVPNOptions, input option.Options) (*option.Options, error) {
	fmt.Printf("config options: %++v\n", opt)

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

	fmt.Print("[BuildConfig] !!! input= \n", input, ",\n  !!! [BuildConfig] ")
	setClashAPI(&options, &opt)
	setLog(&options, &opt)
	setInbound(&options, &opt)
	setDns(&options, &opt)
	fmt.Printf("[debug] Region=%q BlockAds=%v EnableDNSRouting=%v ConnTestUrl=%q\n",
		opt.Region, opt.BlockAds, opt.EnableDNSRouting, opt.ConnectionTestUrl)
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
	fmt.Println("[addForceDirect] !!! \n", directDomains, "\n !!! [addForceDirect]")

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
	dnsRule := option.DefaultDNSRule{
		RawDefaultDNSRule: option.RawDefaultDNSRule{
			Domain: directDomains, // <— массив доменов (например, ["rostovvpn.run.place"])
		},
		DNSRuleAction: option.DNSRuleAction{
			Action: C.RuleActionTypeRoute,
			RouteOptions: option.DNSRouteActionOptions{
				Server: DNSDirectTag, // dns-direct
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

	if opt.Warp.EnableWarp {
		for _, baseOutbound := range input.Outbounds {
			obj, err := outboundToMap(baseOutbound)
			if err != nil {
				return err
			}
			if strings.EqualFold(obj.string("type"), C.TypeWireGuard) {
				privateKey := obj.string("private_key")
				if privateKey == opt.Warp.WireguardConfig.PrivateKey || privateKey == "p1" {
					opt.Warp.EnableWarp = false
					break
				}
			}
			if warpMap, ok := obj.nestedMap("warp"); ok {
				if warpMap.string("key") == "p1" {
					opt.Warp.EnableWarp = false
					break
				}
			}
		}
	}

	if opt.Warp.EnableWarp && (opt.Warp.Mode == "warp_over_proxy" || opt.Warp.Mode == "proxy_over_warp") {
		warpOutbound, err := GenerateWarpSingbox(opt.Warp.WireguardConfig, opt.Warp.CleanIP, opt.Warp.CleanPort, opt.Warp.FakePackets, opt.Warp.FakePacketSize, opt.Warp.FakePacketDelay, opt.Warp.FakePacketMode)
		if err != nil {
			return fmt.Errorf("failed to generate warp config: %v", err)
		}
		warpMap, err := outboundToMap(*warpOutbound)
		if err != nil {
			return err
		}
		warpTag := warpMap.string("tag")
		if warpTag == "" {
			warpTag = "rostovvpn-warp"
		}
		warpMap["tag"] = warpTag
		if opt.Warp.Mode == "warp_over_proxy" {
			warpMap["detour"] = OutboundSelectTag
			OutboundMainProxyTag = warpTag
		} else {
			warpMap["detour"] = OutboundDirectTag
		}
		warpMap, err = patchWarpMap(warpMap, opt, true, staticIPs)
		if err != nil {
			return err
		}
		warpStruct, err := mapToOutbound(warpMap)
		if err != nil {
			return err
		}
		outbounds = append(outbounds, warpStruct)
	}

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
	defaultSelect := urlTest.Tag
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
			// тут при желании можно что-то чуть-чуть «доподкрутить», не меняя типы
			// например:
			// if v.TLS != nil && v.TLS.UTLS != nil && v.TLS.UTLS.Enabled && v.TLS.UTLS.Fingerprint == "" {
			//     v.TLS.UTLS.Fingerprint = "random"
			// }
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
	options.Log = &option.LogOptions{
		Level:        opt.LogLevel,
		Output:       opt.LogFile,
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

	if opt.EnableTunService {
		ActivateTunnelService(*opt)
	} else if opt.EnableTun {
		// безопасный MTU для TUN по умолчанию
		if opt.MTU == 0 || opt.MTU > 2000 {
			opt.MTU = 1450
		}
		addressList := badoption.Listable[netip.Prefix]{}
		switch opt.IPv6Mode {
		case option.DomainStrategy(dns.DomainStrategyUseIPv4):
			addressList = append(addressList, netip.MustParsePrefix("172.19.0.1/28"))
		case option.DomainStrategy(dns.DomainStrategyUseIPv6):
			addressList = append(addressList, netip.MustParsePrefix("fdfe:dcba:9876::1/126"))
		default:
			addressList = append(addressList,
				netip.MustParsePrefix("172.19.0.1/28"),
				netip.MustParsePrefix("fdfe:dcba:9876::1/126"),
			)
		}

		tunOptions := &option.TunInboundOptions{
			Stack:       opt.TUNStack,
			MTU:         opt.MTU,
			AutoRoute:   true,
			StrictRoute: opt.StrictRoute,
			Address:     addressList,
			InboundOptions: option.InboundOptions{
				SniffEnabled:             true,
				SniffOverrideDestination: false,
				DomainStrategy:           inboundDomainStrategy,
			},
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
}
func setDns(options *option.Options, opt *RostovVPNOptions) {
	dnsOptions := &option.DNSOptions{}
	dnsOptions.Final = DNSRemoteTag
	dnsOptions.DNSClientOptions = option.DNSClientOptions{
		IndependentCache: opt.IndependentDNSCache,
	}
	dnsOptions.Servers = []option.DNSServerOptions{
		// remote (udp://8.8.8.8 из shared_prefs -> "8.8.8.8")
		// newDNSServer(DNSRemoteTag, normalizeDNSAddress(opt.RemoteDnsAddress), DNSDirectTag, opt.RemoteDnsDomainStrategy, ""),
		//  ВАЖНО: удалённый DNS ходит ЧЕРЕЗ proxy (select), чтобы DPI не рвал
		newDNSServer(DNSRemoteTag, normalizeDNSAddress(opt.RemoteDnsAddress), DNSDirectTag, opt.RemoteDnsDomainStrategy, OutboundSelectTag),
		// legacyDNSServer(DNSRemoteTag, normalizeDNSAddress(opt.RemoteDnsAddress), DNSDirectTag, opt.RemoteDnsDomainStrategy, ""),

		// DoH с «анти-ДПИ» (оставляем legacy https + детур на direct)
		// legacyDNSServer(DNSTricksDirectTag, "https://sky.rethinkdns.com/", "", opt.DirectDnsDomainStrategy, OutboundDirectTag),
		newDNSServer(DNSTricksDirectTag, "https://sky.rethinkdns.com/", DNSLocalTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),
		// direct (udp)
		// legacyDNSServer(DNSDirectTag, normalizeDNSAddress(opt.DirectDnsAddress), DNSLocalTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),
		newDNSServer(DNSDirectTag, normalizeDNSAddress(opt.DirectDnsAddress), DNSLocalTag, opt.DirectDnsDomainStrategy, OutboundDirectTag),

		// local/rcode — как было
		// legacyDNSServer(DNSLocalTag, "local", "", 0, OutboundDirectTag),
		newDNSServer(DNSLocalTag, "local", "", 0, OutboundDirectTag),

		// legacyDNSServer(DNSBlockTag, "rcode://success", "", 0, ""),
		// newDNSServer(DNSBlockTag, "rcode://success", "", 0, ""),
	}
	options.DNS = dnsOptions
	fmt.Println("[setDns] !!! \n", dnsOptions, "\n !!! [setDns]")

	// // ВАЖНО: если включены TLS-tricks — прокинем их прямо в DoH (dns-trick-direct)
	// injectTLSTricksIntoDoH(options, opt)
	// ВНИМАНИЕ: upstream xtls/sing-box не поддерживает tls_fragment/tls_padding/mixed_sni_case
	// у DNS-серверов. Чтобы не уронить парсер — ничего не инжектим, но выводим предупреждение,
	// если флаги включены.
	warnIfTLSTricksRequestedButUnsupported(opt)

	if ips := getIPs([]string{"www.speedtest.net", "sky.rethinkdns.com"}); len(ips) > 0 {
		applyStaticIPHosts(options, map[string][]string{"sky.rethinkdns.com": ips})
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
	fmt.Println("[setFakeDns] !!! \n", dnsRule, "\n !!! [setFakeDns]")

	options.DNS.Rules = append(options.DNS.Rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: dnsRule})
}
func setRoutingOptions(options *option.Options, opt *RostovVPNOptions) {
	if options.DNS == nil {
		options.DNS = &option.DNSOptions{}
	}

	dnsRules := make([]option.DNSRule, 0)
	routeRules := make([]option.Rule, 0)
	rulesets := make([]option.RuleSet, 0)

	if opt.EnableTun && runtime.GOOS == "android" {
		match := option.RawDefaultRule{
			Inbound:     []string{InboundTUNTag},
			PackageName: []string{"app.rostovvpn.com"},
		}
		routeRules = append(routeRules, newRouteRule(match, OutboundBypassTag))
	}

	routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{Inbound: []string{InboundDNSTag}}, OutboundDNSTag))
	routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{Port: []uint16{53}}, OutboundDNSTag))

	if opt.BypassLAN {
		routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{IPIsPrivate: true}, OutboundBypassTag))
	}

	// A) Любой трафик к DoH-хосту — напрямую (direct)
	routeRules = append(routeRules, newRouteRule(
		option.RawDefaultRule{Domain: []string{"sky.rethinkdns.com"}},
		OutboundDirectTag,
	))

	// B) Сами DNS-запросы на этот домен — через наш "dns-trick-direct"
	dnsRules = append(dnsRules, newDNSRouteRule(
		option.DefaultDNSRule{
			RawDefaultDNSRule: option.RawDefaultDNSRule{
				Domain: []string{"sky.rethinkdns.com"},
			},
		},
		DNSTricksDirectTag, // tag сервера из setDns()
	))

	fmt.Println("[setRoutingOptions] !!! dnsRules=\n", dnsRules, "]n !!! [setRoutingOptions]")

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
			server = DNSDirectTag
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
	fmt.Println("[setRoutingOptions] !!! \n", *opt, "]n !!! [setRoutingOptions]")
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
		dnsRules = append(dnsRules, newDNSRouteRule(
			option.DefaultDNSRule{
				RawDefaultDNSRule: option.RawDefaultDNSRule{
					DomainSuffix: []string{"." + opt.Region},
				},
			},
			DNSDirectTag,
		))
		fmt.Println("[setRoutingOptions] not other!!! \n", opt.Region, "\n\n\n\n\n", dnsRules, "]n !!! [setRoutingOptions]")

		routeRules = append(routeRules, newRouteRule(option.RawDefaultRule{RuleSet: regionTags}, OutboundDirectTag))
		dnsRules = append(dnsRules, newDNSRouteRule(option.DefaultDNSRule{RawDefaultDNSRule: option.RawDefaultDNSRule{RuleSet: regionTags}}, DNSDirectTag))
	}
	if options.Route == nil {
		options.Route = &option.RouteOptions{
			Final:               OutboundMainProxyTag,
			AutoDetectInterface: true,
			OverrideAndroidVPN:  runtime.GOOS == "android",
			DefaultDomainResolver: &option.DomainResolveOptions{
				Server: pickDefaultResolver(opt),
			},
		}
	} else {
		if options.Route.Final == "" {
			options.Route.Final = OutboundMainProxyTag
		}
		options.Route.AutoDetectInterface = true
		options.Route.OverrideAndroidVPN = runtime.GOOS == "android"
		if options.Route.DefaultDomainResolver == nil {
			options.Route.DefaultDomainResolver = &option.DomainResolveOptions{Server: pickDefaultResolver(opt)}
		} else if options.Route.DefaultDomainResolver.Server == "" {
			options.Route.DefaultDomainResolver.Server = pickDefaultResolver(opt)
		}
	}

	// Не задаём DefaultNetworkStrategy, если пользователь не попросил.
	if ns := toNetworkStrategyPtr(opt.DefaultNetworkStrategy); ns != nil {
		options.Route.DefaultNetworkStrategy = ns
	} /* else {
	    // Если хочешь «автоматическое наследование» из IPv6Mode — раскоммить маппер ниже.
	    if s, ok := mapDomainToNetworkStrategy(opt.IPv6Mode); ok {
	        options.Route.DefaultNetworkStrategy = s
	    }
	} */
	fmt.Println("[setRoutingOptions] !!! Rules\n", routeRules, "\n !!! [setRoutingOptions]")

	options.Route.Rules = append(options.Route.Rules, routeRules...)
	options.Route.RuleSet = append(options.Route.RuleSet, rulesets...)

	if opt.EnableDNSRouting {
		fmt.Println("[setRoutingOptions] !!! dnsRules\n", dnsRules, "\n !!! [setRoutingOptions]")

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
	return DNSDirectTag
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

// func newDNSServer(tag, address, resolver string, strategy option.DomainStrategy, detour string) option.DNSServerOptions {
// 	p, err := ParseDNSAddr(address)
// 	fmt.Println("[newDNSServer] !!! \n\n\n\n", p, "\n\n\n\n !!! [newDNSServer]")
// 	if err != nil {
// 		return legacyDNSServer(tag, address, resolver, strategy, detour)
// 	}
// 	obj := map[string]any{}
// 	if p.Host != "" {
// 		obj["server"] = p.Host
// 	}
// 	if resolver != "" || strategy != 0 {
// 		if resolver == "" {
// 			resolver = DNSLocalTag
// 		}
// 		dr := map[string]any{"server": resolver}
// 		if strategy != 0 {
// 			dr["strategy"] = strategy
// 		}
// 		obj["domain_resolver"] = dr
// 	}
// 	if detour != "" && detour != "direct" && detour != "dns-trick-direct" && detour != "dns-direct" && detour != "local" {
// 		obj["detour"] = detour
// 	}
// 	if p.Port != 0 && p.Port != 53 {
// 		obj["server_port"] = p.Port
// 	}
// 	return option.DNSServerOptions{
// 		Type:    p.Scheme,
// 		Tag:     tag,
// 		Options: obj,
// 	}
// }

func newRemoteRuleSet(tag, url string) option.RuleSet {
	return option.RuleSet{
		Type:   C.RuleSetTypeRemote,
		Tag:    tag,
		Format: C.RuleSetFormatBinary,
		RemoteOptions: option.RemoteRuleSet{
			URL:            url,
			UpdateInterval: badoption.Duration(5 * 24 * time.Hour),
			DownloadDetour: OutboundSelectTag,
		},
	}
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

// Если понадобится попробовать схему через объект TLS (вдруг твой форк это умеет),
// то можно заменить warnIfTLSTricksRequestedButUnsupported() на этот вариант:
//
// func tryInjectTLSTricksViaTLSObject(options *option.Options, opt *RostovVPNOptions) {
//     if options == nil || options.DNS == nil || !hasTLSTricks(opt) {
//         return
//     }
//     for i := range options.DNS.Servers {
//         s := &options.DNS.Servers[i]
//         if s.Tag != DNSTricksDirectTag || (s.Type != "https" && s.Type != "h3") {
//             continue
//         }
//         mm, ok := s.Options.(map[string]any); if !ok { continue }
//         // ПРЕДУПРЕЖДЕНИЕ: если парсер не знает эти поля внутри "tls" — конфиг упадёт.
//         tlsObj := map[string]any{"enabled": true}
//         // Ниже ключи примерные; включай, только если точно поддерживаются сборкой форка.
//         if opt.TLSTricks.MixedSNICase { tlsObj["mixed_case_sni"] = true }
//         if opt.TLSTricks.EnableFragment {
//             frag := map[string]any{}
//             if v := strings.TrimSpace(opt.TLSTricks.FragmentSize); v != "" { frag["size"] = v }
//             if v := strings.TrimSpace(opt.TLSTricks.FragmentSleep); v != "" { frag["sleep"] = v }
//             if len(frag) > 0 { tlsObj["fragment"] = frag }
//         }
//         if opt.TLSTricks.EnablePadding {
//             pad := map[string]any{}
//             if v := strings.TrimSpace(opt.TLSTricks.PaddingSize); v != "" { pad["size"] = v }
//             if len(pad) > 0 { tlsObj["padding"] = pad }
//         }
//         // merge
//         mm["tls"] = tlsObj
//         s.Options = mm
//     }
// }
// ----- /TLS tricks -----

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
	fmt.Println("[applyStaticIPHosts] !!! dnsRule\n", dnsRule, "\n !!! [applyStaticIPHosts]")

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

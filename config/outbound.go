package config

import (
	"fmt"
	"net"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	option "github.com/sagernet/sing-box/option"
)

func patchOutboundMux(obj outboundMap, configOpt RostovVPNOptions) outboundMap {
	if !configOpt.Mux.Enable {
		return obj
	}
	switch obj.string("type") {
	case C.TypeSelector, C.TypeURLTest, C.TypeDirect, C.TypeBlock, C.TypeDNS:
		return obj
	}
	obj["multiplex"] = map[string]any{
		"enabled":     true,
		"padding":     configOpt.Mux.Padding,
		"max_streams": configOpt.Mux.MaxStreams,
		"protocol":    configOpt.Mux.Protocol,
	}
	return obj
}

func patchOutboundFragment(obj outboundMap, configOpt RostovVPNOptions) outboundMap {
	if !configOpt.TLSTricks.EnableFragment {
		return obj
	}
	obj["tcp_fast_open"] = false
	obj["tls_fragment"] = map[string]any{
		"enabled": true,
		"size":    configOpt.TLSTricks.FragmentSize,
		"sleep":   configOpt.TLSTricks.FragmentSleep,
	}
	return obj
}

func patchOutboundTLSTricks(obj outboundMap, configOpt RostovVPNOptions) outboundMap {
	outType := obj.string("type")
	if outType == C.TypeSelector || outType == C.TypeURLTest || outType == C.TypeBlock || outType == C.TypeDNS {
		return obj
	}
	if isOutboundReality(obj) {
		return obj
	}
	if outType == C.TypeDirect {
		return patchOutboundFragment(obj, configOpt)
	}
	tlsMap, ok := obj.nestedMap("tls")
	if !ok || !tlsMap.bool("enabled") {
		return obj
	}
	if transport, ok := obj.nestedMap("transport"); ok {
		typeValue := strings.ToLower(transport.string("type"))
		switch typeValue {
		case C.V2RayTransportTypeWebsocket, C.V2RayTransportTypeGRPC, C.V2RayTransportTypeHTTPUpgrade:
			// continue
		default:
			return obj
		}
	}
	obj = patchOutboundFragment(obj, configOpt)
	tlsTricks := tlsMap.ensureNestedMap("tls_tricks")
	if configOpt.TLSTricks.MixedSNICase {
		tlsTricks["mixed_case_sni"] = true
	}
	if configOpt.TLSTricks.EnablePadding {
		tlsTricks["padding_mode"] = "random"
		tlsTricks["padding_size"] = configOpt.TLSTricks.PaddingSize
		utls := tlsMap.ensureNestedMap("utls")
		utls["enabled"] = true
		utls["fingerprint"] = "custom"
	}
	return obj
}

func isOutboundReality(obj outboundMap) bool {
	if obj.string("type") != C.TypeVLESS {
		return false
	}
	tlsMap, ok := obj.nestedMap("tls")
	if !ok {
		return false
	}
	reality, ok := tlsMap.nestedMap("reality")
	if !ok {
		return false
	}
	return reality.bool("enabled")
}

func detectServerDomain(obj outboundMap) string {
	if detour := obj.string("detour"); detour != "" {
		return ""
	}
	server := obj.string("server")
	if server == "" || net.ParseIP(server) != nil {
		return ""
	}
	return fmt.Sprintf("full:%s", server)
}

func patchOutbound(base option.Outbound, configOpt RostovVPNOptions, staticIPs map[string][]string) (*option.Outbound, string, error) {
	formatErr := func(err error) error {
		return fmt.Errorf("error patching outbound[%s][%s]: %w", base.Tag, base.Type, err)
	}
	obj, err := outboundToMap(base)
	if err != nil {
		return nil, "", formatErr(err)
	}
	obj, err = patchWarpMap(obj, &configOpt, true, staticIPs)
	if err != nil {
		return nil, "", formatErr(err)
	}
	serverDomain := detectServerDomain(obj)
	obj = patchOutboundTLSTricks(obj, configOpt)
	switch obj.string("type") {
	case C.TypeVMess, C.TypeVLESS, C.TypeTrojan, C.TypeShadowsocks:
		obj = patchOutboundMux(obj, configOpt)
	}
	outbound, err := mapToOutbound(obj)
	if err != nil {
		return nil, "", formatErr(err)
	}
	return &outbound, serverDomain, nil
}

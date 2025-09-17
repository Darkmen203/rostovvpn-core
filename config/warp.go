package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Darkmen203/rostovvpn-core/v2/common"
	"github.com/bepass-org/warp-plus/warp"
	C "github.com/sagernet/sing-box/constant"

	// "github.com/bepass-org/wireguard-go/warp"
	"github.com/Darkmen203/rostovvpn-core/v2/db"

	option "github.com/sagernet/sing-box/option"
)

type SingboxConfig struct {
	Type          string   `json:"type"`
	Tag           string   `json:"tag"`
	Server        string   `json:"server"`
	ServerPort    int      `json:"server_port"`
	LocalAddress  []string `json:"local_address"`
	PrivateKey    string   `json:"private_key"`
	PeerPublicKey string   `json:"peer_public_key"`
	Reserved      []int    `json:"reserved"`
	MTU           int      `json:"mtu"`
}

func wireGuardConfigToMap(wgConfig WarpWireguardConfig, server string, port uint16) (outboundMap, error) {
	clientID, _ := base64.StdEncoding.DecodeString(wgConfig.ClientID)
	if len(clientID) < 3 {
		clientID = append(clientID, make([]byte, 3-len(clientID))...)
	}
	reserved := []int{0, 0, 0}
	for i := 0; i < len(reserved) && i < len(clientID); i++ {
		reserved[i] = int(clientID[i])
	}
	obj := outboundMap{
		"type":            C.TypeWireGuard,
		"tag":             "WARP",
		"server":          server,
		"server_port":     int(port),
		"private_key":     wgConfig.PrivateKey,
		"peer_public_key": wgConfig.PeerPublicKey,
		"reserved":        reserved,
		"mtu":             1330,
	}
	var addresses []string
	if wgConfig.LocalAddressIPv4 != "" {
		addresses = append(addresses, wgConfig.LocalAddressIPv4+"/24")
	}
	if wgConfig.LocalAddressIPv6 != "" {
		addresses = append(addresses, wgConfig.LocalAddressIPv6+"/128")
	}
	if len(addresses) > 0 {
		obj["local_address"] = addresses
	}
	return obj, nil
}

func getRandomIP() string {
	ipPort, err := warp.RandomWarpEndpoint(true, true)
	if err == nil {
		return ipPort.Addr().String()
	}
	return "engage.cloudflareclient.com"
}

func generateWarp(license string, host string, port uint16, fakePackets string, fakePacketsSize string, fakePacketsDelay string, fakePacketsMode string) (*option.Outbound, error) {
	_, _, wgConfig, err := GenerateWarpInfo(license, "", "")
	if err != nil {
		return nil, err
	}
	if wgConfig == nil {
		return nil, fmt.Errorf("invalid warp config")
	}

	return GenerateWarpSingbox(*wgConfig, host, port, fakePackets, fakePacketsSize, fakePacketsDelay, fakePacketsMode)
}

func GenerateWarpSingbox(wgConfig WarpWireguardConfig, host string, port uint16, fakePackets string, fakePacketsSize string, fakePacketsDelay string, fakePacketMode string) (*option.Outbound, error) {
	if host == "" {
		host = "auto4"
	}

	if (host == "auto" || host == "auto4" || host == "auto6") && fakePackets == "" {
		fakePackets = "1-3"
	}
	if fakePackets != "" && fakePacketsSize == "" {
		fakePacketsSize = "10-30"
	}
	if fakePackets != "" && fakePacketsDelay == "" {
		fakePacketsDelay = "10-30"
	}
	obj, err := wireGuardConfigToMap(wgConfig, host, port)
	if err != nil {
		fmt.Printf("%v %v", obj, err)
		return nil, err
	}
	if fakePackets != "" {
		obj["fake_packets"] = fakePackets
	}
	if fakePacketsSize != "" {
		obj["fake_packets_size"] = fakePacketsSize
	}
	if fakePacketsDelay != "" {
		obj["fake_packets_delay"] = fakePacketsDelay
	}
	if fakePacketMode != "" {
		obj["fake_packets_mode"] = fakePacketMode
	}
	outbound, err := mapToOutbound(obj)
	if err != nil {
		return nil, err
	}
	return &outbound, nil
}

func GenerateWarpInfo(license string, oldAccountId string, oldAccessToken string) (*warp.Identity, string, *WarpWireguardConfig, error) {
	if oldAccountId != "" && oldAccessToken != "" {
		err := warp.DeleteDevice(oldAccessToken, oldAccountId)
		if err != nil {
			fmt.Printf("Error in removing old device: %v\n", err)
		} else {
			fmt.Printf("Old Device Removed")
		}
	}
	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	identity, err := warp.CreateIdentityOnly(l, license)
	res := "Error!"
	var warpcfg WarpWireguardConfig
	if err == nil {
		res = "Success"
		res = fmt.Sprintf("Warp+ enabled: %t\n", identity.Account.WarpPlus)
		res += fmt.Sprintf("\nAccount type: %s\n", identity.Account.AccountType)
		warpcfg = WarpWireguardConfig{
			PrivateKey:       identity.PrivateKey,
			PeerPublicKey:    identity.Config.Peers[0].PublicKey,
			LocalAddressIPv4: identity.Config.Interface.Addresses.V4,
			LocalAddressIPv6: identity.Config.Interface.Addresses.V6,
			ClientID:         identity.Config.ClientID,
		}
	}

	return &identity, res, &warpcfg, err
}

func getOrGenerateWarpLocallyIfNeeded(warpOptions *WarpOptions) WarpWireguardConfig {
	if warpOptions.WireguardConfig.PrivateKey != "" {
		return warpOptions.WireguardConfig
	}
	table := db.GetTable[WarpOptions]()
	dbWarpOptions, err := table.Get(warpOptions.Id)
	if err == nil && dbWarpOptions.WireguardConfig.PrivateKey != "" {
		return warpOptions.WireguardConfig
	}
	license := ""
	if len(warpOptions.Id) == 26 { // warp key is 26 characters long
		license = warpOptions.Id
	} else if len(warpOptions.Id) > 28 && warpOptions.Id[2] == '_' { // warp key is 26 characters long
		license = warpOptions.Id[3:]
	}

	accountidentity, _, wireguardConfig, err := GenerateWarpInfo(license, warpOptions.Account.AccountID, warpOptions.Account.AccessToken)
	if err != nil {
		return WarpWireguardConfig{}
	}
	warpOptions.Account = WarpAccount{
		AccountID:   accountidentity.ID,
		AccessToken: accountidentity.Token,
	}
	warpOptions.WireguardConfig = *wireguardConfig
	table.UpdateInsert(warpOptions)

	return *wireguardConfig
}

func patchWarpMap(obj outboundMap, configOpt *RostovVPNOptions, final bool, staticIPs map[string][]string) (outboundMap, error) {
	if staticIPs == nil {
		staticIPs = make(map[string][]string)
	}
	if warpInfo, ok := obj["warp"].(map[string]any); ok {
		key := ""
		if v, ok := warpInfo["key"].(string); ok {
			key = v
		}
		host := ""
		if v, ok := warpInfo["host"].(string); ok {
			host = v
		}
		var port uint16
		switch value := warpInfo["port"].(type) {
		case float64:
			port = uint16(value)
		case json.Number:
			if iv, err := value.Int64(); err == nil {
				port = uint16(iv)
			}
		}
		detour, _ := warpInfo["detour"].(string)
		fakePackets, _ := warpInfo["fake_packets"].(string)
		fakePacketsSize, _ := warpInfo["fake_packets_size"].(string)
		fakePacketsDelay, _ := warpInfo["fake_packets_delay"].(string)
		fakePacketsMode, _ := warpInfo["fake_packets_mode"].(string)

		isSavedKey := len(key) > 1 && key[0] == 'p'
		if (configOpt == nil || !final) && isSavedKey {
			return obj, nil
		}

		var (
			err             error
			wireguardConfig WarpWireguardConfig
		)
		if isSavedKey {
			var warpOpt *WarpOptions
			switch key {
			case "p1":
				warpOpt = &configOpt.Warp
			case "p2":
				warpOpt = &configOpt.Warp2
			default:
				warpOpt = &WarpOptions{Id: key}
			}
			warpOpt.Id = key
			wireguardConfig = getOrGenerateWarpLocallyIfNeeded(warpOpt)
		} else {
			_, _, wgConfig, genErr := GenerateWarpInfo(key, "", "")
			if genErr != nil {
				return nil, genErr
			}
			wireguardConfig = *wgConfig
		}
		warpOutbound, err := GenerateWarpSingbox(wireguardConfig, host, port, fakePackets, fakePacketsSize, fakePacketsDelay, fakePacketsMode)
		if err != nil {
			fmt.Printf("Error generating warp config: %v", err)
			return nil, err
		}
		warpMap, err := outboundToMap(*warpOutbound)
		if err != nil {
			return nil, err
		}
		currentTag := obj.string("tag")
		for key, value := range warpMap {
			if key == "tag" {
				continue
			}
			obj[key] = value
		}
		if currentTag != "" {
			obj["tag"] = currentTag
		}
		obj["type"] = C.TypeWireGuard
		if detour != "" {
			obj["detour"] = detour
		}
		delete(obj, "warp")
	}

	if final && strings.EqualFold(obj.string("type"), C.TypeWireGuard) {
		host := obj.string("server")
		if host == "default" || host == "random" || host == "auto" || host == "auto4" || host == "auto6" || isBlockedDomain(host) {
			rndDomain := strings.ToLower(generateRandomString(20))
			staticIPs[rndDomain] = []string{}
			if host != "auto4" {
				if host == "auto6" || common.CanConnectIPv6() {
					randomIP, _ := warp.RandomWarpEndpoint(false, true)
					staticIPs[rndDomain] = append(staticIPs[rndDomain], randomIP.Addr().String())
				}
			}
			if host != "auto6" {
				randomIP, _ := warp.RandomWarpEndpoint(true, false)
				staticIPs[rndDomain] = append(staticIPs[rndDomain], randomIP.Addr().String())
			}
			obj["server"] = rndDomain
		}
		if portValue, ok := obj.float64("server_port"); !ok || portValue == 0 {
			obj["server_port"] = warp.RandomWarpPort()
		}
		if detour := obj.string("detour"); detour != "" {
			if mtuValue, ok := obj.float64("mtu"); !ok || mtuValue < 100 {
				obj["mtu"] = 1280
			}
			obj["fake_packets"] = ""
			obj["fake_packets_delay"] = ""
			obj["fake_packets_size"] = ""
		}
	}
	return obj, nil
}

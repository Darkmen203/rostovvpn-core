module github.com/Darkmen203/rostovvpn-core

go 1.25

toolchain go1.25.1

require (
	// Ниже — все «индирект» библиотеки, которые вы перечислили.
	// Go может удалить часть из них при go mod tidy, если не используются в коде.
	github.com/DataDog/zstd v1.4.1 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect

	// Из строки "github.com/bepass-org/warp-plus v1.2.4"
	github.com/bepass-org/warp-plus v1.2.4
	github.com/caddyserver/certmagic v0.23.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cosmos/gorocksdb v1.2.0 // indirect
	github.com/cretz/bine v0.2.0 // indirect
	github.com/desertbit/timer v0.0.0-20180107155436-c41aec40b27f // indirect
	github.com/dgraph-io/badger/v2 v2.2007.2 // indirect
	github.com/dgraph-io/ristretto v0.0.3-0.20200630154024-f66de99634de // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-chi/chi/v5 v5.2.2 // indirect
	github.com/go-chi/render v1.0.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gofrs/uuid/v5 v5.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.1 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/improbable-eng/grpc-web v0.15.0
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/insomniacslk/dhcp v0.0.0-20250417080101-5f8cf70e8c5f // indirect
	github.com/jellydator/validation v1.1.0
	github.com/jmhodges/levigo v1.0.0 // indirect
	github.com/kardianos/service v1.2.2
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/libdns/alidns v1.0.5-libdns.v1.beta1 // indirect
	github.com/libdns/cloudflare v0.2.2-0.20250708034226-c574dccb31a6 // indirect
	github.com/libdns/libdns v1.1.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.68 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/refraction-networking/utls v1.8.0 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/sagernet/bbolt v0.0.0-20231014093535-ea5cb2fe9f0a // indirect
	github.com/sagernet/gomobile v0.1.8
	github.com/sagernet/gvisor v0.0.0-20250811-sing-box-mod.1 // indirect
	github.com/sagernet/netlink v0.0.0-20240612041022-b9a21c07ac6a // indirect
	github.com/sagernet/quic-go v0.54.0-sing-box-mod.2 // indirect
	github.com/sagernet/sing v0.8.0-beta.2
	github.com/sagernet/sing-box v1.12.8
	github.com/sagernet/sing-dns v0.2.3
	github.com/sagernet/sing-mux v0.3.3 // indirect
	github.com/sagernet/sing-quic v0.6.0-beta.2 // indirect
	github.com/sagernet/sing-shadowsocks v0.2.9 // indirect
	github.com/sagernet/sing-shadowsocks2 v0.2.1 // indirect
	github.com/sagernet/sing-shadowtls v0.2.1-0.20250503051639-fcd445d33c11 // indirect
	github.com/sagernet/sing-tun v0.8.0-beta.10 // indirect
	github.com/sagernet/sing-vmess v0.2.8-0.20250909125414-3aed155119a1 // indirect
	github.com/sagernet/smux v1.5.34-mod.2 // indirect
	github.com/sagernet/wireguard-go v0.0.2-beta.1.0.20250917110311-16510ac47288 // indirect
	github.com/sagernet/ws v0.0.0-20231204124109-acfe8907c854 // indirect
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	github.com/tendermint/tm-db v0.6.7
	github.com/u-root/uio v0.0.0-20240224005618-d2acac8f3701 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/xmdhs/clash2singbox v0.0.2
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.etcd.io/bbolt v1.3.11 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/exp v0.0.0-20250506013437-ce4c2cf36ca6 // indirect
	golang.org/x/mod v0.28.0 // indirect
	golang.org/x/net v0.44.0
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0
	golang.org/x/text v0.29.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/grpc v1.75.1
	google.golang.org/protobuf v1.36.9
	gopkg.in/yaml.v3 v3.0.1
	lukechampine.com/blake3 v1.4.1 // indirect
	nhooyr.io/websocket v1.8.6 // indirect
)

require (
	github.com/Darkmen203/ray2sing v0.0.0-20250917072930-914f270bd17c
	github.com/Darkmen203/rostovvpn-app-demo-extension v0.0.0-20250917202306-2e4594ab2e05
	github.com/Darkmen203/rostovvpn-ip-scanner-extension v0.0.0-20250917202754-03bcaee72452
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/akutz/memconn v0.1.0 // indirect
	github.com/alexbrainman/sspi v0.0.0-20231016080023-1a75b4708caa // indirect
	github.com/anytls/sing-anytls v0.0.8 // indirect
	github.com/caddyserver/zerossl v0.1.3 // indirect
	github.com/coder/websocket v1.8.13 // indirect
	github.com/coreos/go-iptables v0.7.1-0.20240112124308-65c67c9f46e6 // indirect
	github.com/database64128/netx-go v0.0.0-20240905055117-62795b8b054a // indirect
	github.com/database64128/tfo-go/v2 v2.2.2 // indirect
	github.com/dblohm7/wingoes v0.0.0-20240119213807-a09d6be7affa // indirect
	github.com/digitalocean/go-smbios v0.0.0-20180907143718-390a4f403a8e // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/gaissmai/bart v0.18.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20250223041408-d3c622f1b874 // indirect
	github.com/godbus/dbus/v5 v5.1.1-0.20230522191255-76236955d466 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/nftables v0.2.1-0.20240414091927-5e242ec57806 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hdevalence/ed25519consensus v0.2.0 // indirect
	github.com/illarion/gonotify/v3 v3.0.2 // indirect
	github.com/imkira/go-observer/v2 v2.0.0-20230629064422-8e0b61f11f1b
	github.com/jsimonetti/rtnetlink v1.4.0 // indirect
	github.com/mdlayher/genetlink v1.3.2 // indirect
	github.com/mdlayher/netlink v1.7.3-0.20250113171957-fbb4dce95f42 // indirect
	github.com/mdlayher/sdnotify v1.0.0 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/metacubex/utls v1.8.0 // indirect
	github.com/mholt/acmez/v3 v3.1.2 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/onsi/gomega v1.33.1 // indirect
	github.com/prometheus-community/pro-bing v0.4.0 // indirect
	github.com/rodaine/table v1.1.1 // indirect
	github.com/safchain/ethtool v0.3.0 // indirect
	github.com/sagernet/cors v1.2.1 // indirect
	github.com/sagernet/fswatch v0.1.1 // indirect
	github.com/sagernet/nftables v0.3.0-beta.4 // indirect
	github.com/sagernet/tailscale v1.86.5-sing-box-1.13-mod.3 // indirect
	github.com/tailscale/certstore v0.1.1-0.20231202035212-d3fa0460f47e // indirect
	github.com/tailscale/go-winio v0.0.0-20231025203758-c4f33415bf55 // indirect
	github.com/tailscale/goupnp v1.0.1-0.20210804011211-c64d0f06ea05 // indirect
	github.com/tailscale/hujson v0.0.0-20221223112325-20486734a56a // indirect
	github.com/tailscale/netlink v1.1.1-0.20240822203006-4d49adab4de7 // indirect
	github.com/tailscale/peercred v0.0.0-20250107143737-35a0c7bd7edc // indirect
	github.com/tailscale/web-client-prebuilt v0.0.0-20250124233751-d4cd19a26976 // indirect
	github.com/tailscale/wireguard-go v0.0.0-20250716170648-1d0488a3d7da // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/zap/exp v0.3.0 // indirect
	go4.org/mem v0.0.0-20240501181205-ae6ca9944745 // indirect
	golang.org/x/term v0.35.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	golang.zx2c4.com/wireguard/windows v0.5.3 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/sagernet/sing-box => github.com/Darkmen203/rostovvpn-sing-box v0.0.0-20250923160250-118b983a614d

replace github.com/xtls/xray-core => github.com/Darkmen203/xray-core v0.0.0-20250917075812-85bb28f203ae

replace github.com/sagernet/wireguard-go => github.com/sagernet/wireguard-go v0.0.2-beta.1

replace github.com/bepass-org/warp-plus => github.com/Darkmen203/warp-plus v0.0.0-20250914174007-d0abc5061784

replace github.com/syndtr/goleveldb => github.com/syndtr/goleveldb v1.0.0

replace github.com/Darkmen203/ray2sing => github.com/Darkmen203/ray2sing v0.0.0-20250917072930-914f270bd17c

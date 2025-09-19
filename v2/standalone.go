package v2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Darkmen203/rostovvpn-core/config"
	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func RunStandalone(rostovvpnSettingPath string, configPath string, defaultConfig config.RostovVPNOptions) error {
	fmt.Println("Running in standalone mode")
	useFlutterBridge = false
	current, err := readAndBuildConfig(rostovvpnSettingPath, configPath, &defaultConfig)
	if err != nil {
		fmt.Printf("Error in read and build config %v", err)
		return err
	}

	go StartService(&pb.StartRequest{
		ConfigContent:          current.Config,
		EnableOldCommandServer: false,
		DelayStart:             false,
		EnableRawConfig:        true,
	})
	go updateConfigInterval(current, rostovvpnSettingPath, configPath)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Waiting for CTRL+C to stop\n")
	<-sigChan
	fmt.Printf("CTRL+C recived-->stopping\n")
	_, err = Stop()

	return err
}

type ConfigResult struct {
	Config                    string
	RefreshInterval           int
	RostovvpnRostovVPNOptions *config.RostovVPNOptions
}

func readAndBuildConfig(rostovvpnSettingPath string, configPath string, defaultConfig *config.RostovVPNOptions) (ConfigResult, error) {
	var result ConfigResult

	fmt.Println("[standalone.readAndBuildConfig] !!! ", rostovvpnSettingPath, " !!! [standalone.readAndBuildConfig]")
	fmt.Println("[standalone.readAndBuildConfig] !!! defaultConfig= \n", defaultConfig, "\n !!! [standalone.readAndBuildConfig]")
	result, err := readConfigContent(configPath)
	fmt.Println("[readAndBuildConfig] !!! [readConfigContent ] result= \n", result, "\n !!! [readAndBuildConfig] !!! [readConfigContent ] ")
	if err != nil {
		return result, err
	}

	// База — дефолты; сверху накроем defaultConfig (если есть) и shared_prefs (если путь задан)
	rostovvpnconfig := config.DefaultRostovVPNOptions()
	if defaultConfig != nil {
		*rostovvpnconfig = *defaultConfig
	}
	fmt.Println("[readAndBuildConfig] !!! [DefaultRostovVPNOptions ] rostovvpnconfig= \n", rostovvpnconfig, "\n !!! [readAndBuildConfig] !!! [DefaultRostovVPNOptions ] ")

	if rostovvpnSettingPath != "" {
		rostovvpnconfig, err = ReadRostovVPNOptionsAt(rostovvpnSettingPath)
		if err != nil {
			return result, err
		}
	}

	result.RostovvpnRostovVPNOptions = rostovvpnconfig
	fmt.Println("[readAndBuildConfig] !!! [before result.Config ] result= \n", result.Config, ",\n  !!! [readAndBuildConfig] ")
	result.Config, err = buildConfig(result.Config, *rostovvpnconfig)

	if err != nil {
		return result, err
	}

	return result, nil
}

func readConfigContent(configPath string) (ConfigResult, error) {
	var content string
	var refreshInterval int

	if strings.HasPrefix(configPath, "http://") || strings.HasPrefix(configPath, "https://") {
		client := &http.Client{}

		// Create a new request
		req, err := http.NewRequest("GET", configPath, nil)
		if err != nil {
			fmt.Println("Error creating request:", err)
			return ConfigResult{}, err
		}
		req.Header.Set("User-Agent", "RostovVPN/2.3.1 ("+runtime.GOOS+") like ClashMeta v2ray sing-box")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error making GET request:", err)
			return ConfigResult{}, err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return ConfigResult{}, fmt.Errorf("failed to read config body: %w", err)
		}
		content = string(body)
		refreshInterval, _ = extractRefreshInterval(resp.Header, content)
		fmt.Printf("Refresh interval: %d\n", refreshInterval)
	} else {
		data, err := ioutil.ReadFile(configPath)
		if err != nil {
			return ConfigResult{}, fmt.Errorf("failed to read config file: %w", err)
		}
		content = string(data)
		fmt.Println("[standalone.readConfigContent] !!! content= \n", content, "\n !!! [standalone.readConfigContent]")
	}

	return ConfigResult{
		Config:          content,
		RefreshInterval: refreshInterval,
	}, nil
}

func extractRefreshInterval(header http.Header, bodyStr string) (int, error) {
	refreshIntervalStr := header.Get("profile-update-interval")
	if refreshIntervalStr != "" {
		refreshInterval, err := strconv.Atoi(refreshIntervalStr)
		if err != nil {
			return 0, fmt.Errorf("failed to parse refresh interval from header: %w", err)
		}
		return refreshInterval, nil
	}

	lines := strings.Split(bodyStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//profile-update-interval:") || strings.HasPrefix(line, "#profile-update-interval:") {
			parts := strings.SplitN(line, ":", 2)
			str := strings.TrimSpace(parts[1])
			refreshInterval, err := strconv.Atoi(str)
			if err != nil {
				return 0, fmt.Errorf("failed to parse refresh interval from body: %w", err)
			}
			return refreshInterval, nil
		}
	}
	return 0, nil
}

func buildConfig(configContent string, options config.RostovVPNOptions) (string, error) {

	parsedContent, err := config.ParseConfigContent(configContent, true, &options, false)
	fmt.Println("[standalone.buildConfig] !!! [ParseConfigContent] parsedContent= \n", parsedContent, "\n !!! [standalone.buildConfig]")
	if err != nil {
		return "", fmt.Errorf("failed to parse config content: %w", err)
	}

	singconfigs, err := readConfigBytes([]byte(parsedContent))
	// fmt.Print("\n[config.buildConfig] !!! singconfigs= \n", singconfigs, ",\n  !!! [config.buildConfig] \n")
	if err != nil {
		return "", err
	}

	finalconfig, err := config.BuildConfig(options, *singconfigs)
	if err != nil {
		return "", fmt.Errorf("failed to build config: %w", err)
	}

	// Не затираем лог-файл, если он задан
	if options.LogFile == "" {
		finalconfig.Log.Output = ""
	}

	// Уважать уже заданный experimental.clash_api; подставлять дефолты только если пусто
	if finalconfig.Experimental != nil && finalconfig.Experimental.ClashAPI != nil {
		ca := finalconfig.Experimental.ClashAPI
		if ca.ExternalUI == "" {
			ca.ExternalUI = "webui"
		}
		if ca.ExternalController == "" {
			host := "127.0.0.1"
			if options.AllowConnectionFromLAN {
				host = "0.0.0.0"
			}
			port := options.ClashApiPort
			if port == 0 {
				port = 16756
			}
			ca.ExternalController = fmt.Sprintf("%s:%d", host, port)
		}
		if ca.Secret == "" {
			fmt.Print("[standalone.buildConfig] !!!", options.ClashApiSecret," !!! [standalone.buildConfig]")
			if options.ClashApiSecret == "" {
				options.ClashApiSecret = generateRandomString(16) // или твоя функция
			}
			ca.Secret = options.ClashApiSecret
		}
		// Печатаем URL без хардкода 6756
		host, port, _ := net.SplitHostPort(ca.ExternalController)
		if host == "" {
			host = "127.0.0.1"
		}
		fmt.Printf("Open http://%s:%s/ui/?secret=%s in your browser\n", host, port, ca.Secret)
	}

	if err := Setup("./", "./", "./tmp", 0, true); err != nil {
		return "", fmt.Errorf("failed to set up global configuration: %w", err)
	}

	configStr, err := config.ToJson(*finalconfig)

	// --- DEBUG: показать DNS-сервера и кусок финального конфига ---
	fmt.Println("---- FINAL DNS servers ----")
	var inspect struct {
		DNS struct {
			Servers []map[string]any `json:"servers"`
		} `json:"dns"`
	}
	_ = json.Unmarshal([]byte(configStr), &inspect)
	for i, s := range inspect.DNS.Servers {
		fmt.Printf("[%d] tag=%v type=%v address=%v detour=%v resolver=%v\n",
			i, s["tag"], s["type"], s["address"], s["detour"], s["address_resolver"])
	}
	fmt.Println("---- FINAL (first 2KB) ----")
	if len(configStr) > 2048 {
		fmt.Println(configStr[:2048])
	} else {
		fmt.Println(configStr)
	}
	fmt.Println("---- /FINAL ----")
	// --- /DEBUG ---

	if err != nil {
		return "", fmt.Errorf("failed to convert config to JSON: %w", err)
	}

	return configStr, nil
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

func updateConfigInterval(current ConfigResult, rostovvpnSettingPath string, configPath string) {
	if current.RefreshInterval <= 0 {
		return
	}

	for {
		<-time.After(time.Duration(current.RefreshInterval) * time.Hour)
		new, err := readAndBuildConfig(rostovvpnSettingPath, configPath, current.RostovvpnRostovVPNOptions)
		if err != nil {
			continue
		}
		if new.Config != current.Config {
			go Stop()
			go StartService(&pb.StartRequest{
				ConfigContent:          new.Config,
				DelayStart:             false,
				EnableOldCommandServer: false,
				DisableMemoryLimit:     false,
				EnableRawConfig:        true,
			})
		}
		current = new
	}
}

func readConfigBytes(content []byte) (*option.Options, error) {
	ctx := libbox.BaseContext(nil)
	parsed, err := singjson.UnmarshalExtendedContext[option.Options](ctx, content)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func ReadRostovVPNOptionsAt(path string) (*config.RostovVPNOptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Нет файла? — вернём дефолты, это не критично для standalone.
		return config.DefaultRostovVPNOptions(), nil
	}

	// 1) Попытка: это уже структурный RostovVPNOptions?
	{
		var opt config.RostovVPNOptions
		if err := json.Unmarshal(data, &opt); err == nil {
			// Эвристика: если хоть что-то «живое» проставлено — считаем валидным.
			if opt.LogLevel != "" || opt.InboundOptions.MixedPort != 0 || opt.Region != "" ||
				opt.DNSOptions.RemoteDnsAddress != "" || opt.RouteOptions.BypassLAN {
				// Разобрать возможные вложенные warp-конфиги-строки
				if opt.Warp.WireguardConfigStr != "" {
					_ = json.Unmarshal([]byte(opt.Warp.WireguardConfigStr), &opt.Warp.WireguardConfig)
				}
				if opt.Warp2.WireguardConfigStr != "" {
					_ = json.Unmarshal([]byte(opt.Warp2.WireguardConfigStr), &opt.Warp2.WireguardConfig)
				}
				return &opt, nil
			}
		}
	}

	// 2) Иначе это Flutter shared_prefs: map[string]any с ключами "flutter.*"
	raw := map[string]any{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Кривой JSON — вернём дефолты, чтобы не падать.
		fmt.Print("[ReadRostovVPNOptionsAt] !!! [Кривой JSON — вернём дефолты, чтобы не падать.] err= \n", err, ",\n  !!! [ReadRostovVPNOptionsAt] ")

		return config.DefaultRostovVPNOptions(), nil
	}
	opt := config.DefaultRostovVPNOptions()
	applyFlutterPrefs(raw, opt)
	return opt, nil
}

// ---- helpers ----
func applyFlutterPrefs(raw map[string]any, opt *config.RostovVPNOptions) {
	// DNS
	if v := str(raw, "flutter.remote-dns-address"); v != "" {
		opt.RemoteDnsAddress = v
	}
	if v := str(raw, "flutter.direct-dns-address"); v != "" {
		opt.DirectDnsAddress = v
	}
	// LAN/Region
	if v, ok := boolean(raw, "flutter.bypass-lan"); ok {
		opt.BypassLAN = v
	}
	if v := str(raw, "flutter.region"); v != "" {
		opt.Region = strings.ToLower(v)
	}
	// Системный прокси — включает mixed-in SetSystemProxy
	if v := str(raw, "flutter.service-mode"); strings.EqualFold(v, "system-proxy") {
		opt.SetSystemProxy = true
	}
	// Лог-левел (если вдруг передаёте из аппки)
	if v := str(raw, "flutter.log-level"); v != "" {
		opt.LogLevel = v
	}
	// Можно добавить маппинги под будущие ключи:
	//   flutter.enable-fake-dns  -> opt.EnableFakeDNS
	//   flutter.default-domain-resolver -> opt.DefaultDomainResolver
	//   flutter.default-network-strategy -> opt.DefaultNetworkStrategy
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		}
	}
	return ""
}
func boolean(m map[string]any, key string) (bool, bool) {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case bool:
			return t, true
		case string:
			// иногда Flutter кладёт "true"/"false" строками
			if strings.EqualFold(t, "true") {
				return true, true
			}
			if strings.EqualFold(t, "false") {
				return false, true
			}
		}
	}
	return false, false
}

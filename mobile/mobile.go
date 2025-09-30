package mobile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Darkmen203/rostovvpn-core/config"
	"github.com/Darkmen203/rostovvpn-core/v2"

	_ "github.com/sagernet/gomobile"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func Setup(baseDir string, workingDir string, tempDir string, debug bool) error {
	return v2.Setup(baseDir, workingDir, tempDir, 0, debug)
	// return v2.Start(17078)
}

func Parse(path string, tempPath string, debug bool) error {
	config, err := config.ParseConfig(tempPath, debug)
	if err != nil {
		return err
	}
	return os.WriteFile(path, config, 0o644)
}

func BuildConfig(path string, RostovVPNOptionsJson string) (string, error) {
	os.Chdir(filepath.Dir(path))
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	ctx := libbox.BaseContext(nil)
	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, fileContent)
	if err != nil {
		return "", err
	}
	RostovVPNOptions := &config.RostovVPNOptions{}
	err = json.Unmarshal([]byte(RostovVPNOptionsJson), RostovVPNOptions)
	if err != nil {
		return "", nil
	}
	fmt.Println("[mobile.BuildConfig] !!! \noptions=\n", options, "\n\nRostovVPNOptions=\n", RostovVPNOptions, "\n !!! [mobile.BuildConfig]")

	return config.BuildConfigJson(*RostovVPNOptions, options)
}

func GenerateWarpConfig(licenseKey string, accountId string, accessToken string) (string, error) {
	return config.GenerateWarpAccount(licenseKey, accountId, accessToken)
}

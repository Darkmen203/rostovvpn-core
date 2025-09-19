package v2

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	runtimeDebug "runtime/debug"
	"time"

	"github.com/Darkmen203/rostovvpn-core/v2/service_manager"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	singjson "github.com/sagernet/sing/common/json"
)

var (
	sWorkingPath          string
	sTempPath             string
	sUserID               int
	sGroupID              int
	statusPropagationPort int64
)

func InitRostovVPNService() error {
	return service_manager.StartServices()
}

func Setup(basePath string, workingPath string, tempPath string, statusPort int64, debug bool) error {
	statusPropagationPort = statusPort
	fmt.Print("[Setup in service.go] !!! ", statusPropagationPort, " !!! [Setup in service.go]")
	if err := libbox.Setup(&libbox.SetupOptions{
		BasePath:        basePath,
		WorkingPath:     workingPath,
		TempPath:        tempPath,
		FixAndroidStack: runtime.GOOS == "android",
	}); err != nil {
		return E.Cause(err, "setup libbox")
	}

	sWorkingPath = workingPath
	if err := os.Chdir(sWorkingPath); err != nil {
		return err
	}
	sTempPath = tempPath
	sUserID = os.Getuid()
	sGroupID = os.Getgid()

	var defaultWriter io.Writer
	if !debug {
		defaultWriter = io.Discard
	}
	factory, err := log.New(log.Options{
		DefaultWriter: defaultWriter,
		BaseTime:      time.Now(),
		Observable:    true,
	})
	coreLogFactory = factory

	if err != nil {
		return E.Cause(err, "create logger")
	}
	return InitRostovVPNService()
}

func NewService(options option.Options) (*libbox.BoxService, error) {
	runtimeDebug.FreeOSMemory()
	content, err := json.Marshal(options)
	if err != nil {
		return nil, E.Cause(err, "encode config")
	}
	service, err := libbox.NewService(string(content), nil)
	if err != nil {
		return nil, E.Cause(err, "create service")
	}
	runtimeDebug.FreeOSMemory()
	return service, nil
}

func readOptions(configContent string) (option.Options, error) {
	ctx := libbox.BaseContext(nil)
	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(configContent))
	if err != nil {
		return option.Options{}, E.Cause(err, "decode config")
	}
	return options, nil
}

package v2

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	runtimeDebug "runtime/debug"
	"time"

	"github.com/Darkmen203/rostovvpn-core/v2/service_manager"

	// sing-box core
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"

	// инфраструктура sing
	E "github.com/sagernet/sing/common/exceptions"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"
	"github.com/sagernet/sing/service/pause"

	// urltest история — как в libbox
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/libbox"

	// IMPORTANT: use libbox context + sing/common/json to decode into typed option structs
	// libbox "github.com/sagernet/sing-box/experimental/libbox"
	B "github.com/sagernet/sing-box"
)

var (
	sWorkingPath          string
	sTempPath             string
	sUserID               int
	sGroupID              int
	statusPropagationPort int64
)

func InitRostovVPNService() error { return service_manager.StartServices() }

func Setup(basePath, workingPath, tempPath string, statusPort int64, debug bool) error {
	statusPropagationPort = statusPort
	useFlutterBridge = statusPort != 0
	tcpConn := runtime.GOOS == "windows" // TODO add TVOS
	FixAndroidStack := runtime.GOOS == "android"
	libboxOpts := libbox.SetupOptions{
		BasePath:        basePath,
		WorkingPath:     workingPath,
		TempPath:        tempPath,
		IsTVOS:          tcpConn,
		FixAndroidStack: FixAndroidStack,
	}

	log.Debug(fmt.Sprintf(
		"v2.Setup setup: tcpConn=%v, libboxOpts=%+v",
		tcpConn, libboxOpts,
	))
	libbox.Setup(&libboxOpts)
	// пути
	sWorkingPath = workingPath
	_ = os.Chdir(sWorkingPath)
	sTempPath = tempPath
	sUserID = os.Getuid()
	sGroupID = os.Getgid()

	// логгер
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

func NewService(opts option.Options) (*libbox.BoxService, error) {
	runtimeDebug.FreeOSMemory()

	base := libbox.BaseContext(nil)         // nil — если не нужно подменять LocalDNS транспорт платформой
	ctx, cancel := context.WithCancel(base) // уже поверх базового контекста
	ctx = filemanager.WithDefault(ctx, sWorkingPath, sTempPath, sUserID, sGroupID)
	urlTestHistoryStorage := urltest.NewHistoryStorage()
	ctx = service.ContextWithPtr(ctx, urlTestHistoryStorage)
	instance, err := B.New(B.Options{
		Context: ctx,
		Options: opts,
	})
	if err != nil {
		cancel()
		return nil, E.Cause(err, "create service")
	}
	runtimeDebug.FreeOSMemory()
	service := libbox.NewBoxService(
		ctx,
		cancel,
		instance,
		service.FromContext[pause.Manager](ctx),
		urlTestHistoryStorage,
	)
	return &service, nil
}

func readOptions(configContent string) (option.Options, error) {
	// Decode JSON into typed sing-box option structs. If we use std json.Unmarshal,
	// nested fields like RemoteDNSServerOptions become map[string]any, which later
	// crashes in dns.RegisterTransport with:
	//   interface conversion: interface {} is map[string]interface {}, not *option.RemoteDNSServerOptions
	ctx := libbox.BaseContext(nil)
	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(configContent))
	if err != nil {
		return option.Options{}, fmt.Errorf("decode config: %w", err)
	}
	return opts, nil
}

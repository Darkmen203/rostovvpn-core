package v2

import (
	"context"
	"fmt"
	"io"
	"os"
	runtimeDebug "runtime/debug"
	"time"

	"github.com/Darkmen203/rostovvpn-core/v2/service_manager"

	// sing-box core
	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"

	// инфраструктура sing
	E "github.com/sagernet/sing/common/exceptions"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"

	// urltest история — как в libbox
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/experimental/libbox"
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

func NewService(opts option.Options) (*CoreService, error) {
	runtimeDebug.FreeOSMemory()

	// База со всеми нужными реестрами (endpoints/transports и т.д.)
	base := libbox.BaseContext(nil)

	// Файловый менеджер — поверх base
	base = filemanager.WithDefault(base, sWorkingPath, sTempPath, sUserID, sGroupID)

	ctx, cancel := context.WithCancel(base)

	hist := urltest.NewHistoryStorage()
	ctx = service.ContextWithPtr(ctx, hist)

	inst, err := box.New(box.Options{
		Context: ctx,
		Options: opts,
	})
	if err != nil {
		cancel()
		return nil, E.Cause(err, "create service")
	}

	// Если нужен хук режимов Clash — можно достать сервер из контекста
	clash := service.FromContext[*clashapi.Server](ctx) // может быть nil

	runtimeDebug.FreeOSMemory()
	return &CoreService{
		ctx:         ctx,
		cancel:      cancel,
		instance:    inst,
		urlHistory:  hist,
		clashServer: clash, // поле типа any/*clashapi.Server — как у тебя в SetService
	}, nil
}

// Запускаем box.Box (название Run, чтобы не пересечься с gRPC Start(ctx, req))
func (s *CoreService) Run() error {
	if s == nil || s.instance == nil {
		return fmt.Errorf("service not initialized")
	}
	return s.instance.Start()
}

func (s *CoreService) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.urlHistory != nil {
		s.urlHistory.Close()
	}
	if s.instance != nil {
		return s.instance.Close()
	}
	return nil
}

func readOptions(configContent string) (option.Options, error) {
	// ВАЖНО: нужен контекст с зарегистрированными реестрами sing-box
	ctx := libbox.BaseContext(nil)
	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(configContent))
	if err != nil {
		return option.Options{}, E.Cause(err, "decode config")
	}
	return options, nil
}

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
	// IMPORTANT: use libbox context + sing/common/json to decode into typed option structs
	// libbox "github.com/sagernet/sing-box/experimental/libbox"
	C "github.com/sagernet/sing-box/constant"
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

	for i, s := range opts.DNS.Servers {
		log.Info("DNS[", i, "] type=", s.Type, " opts=", fmt.Sprintf("%T", s.Options))
	}

	for i, ob := range opts.Outbounds {
		// Страховка от старой пустышки:
		if fmt.Sprintf("%T", ob.Options) == "config.emptyJSON" {
			return nil, fmt.Errorf("outbound %d (%s): untyped options (config.emptyJSON) — замените на StubOptions/DirectOutboundOptions или nil", i, ob.Tag)
		}

		switch ob.Type {
		case C.TypeVLESS:
			if _, ok := ob.Options.(*option.VLESSOutboundOptions); !ok {
				return nil, fmt.Errorf("outbound %d (%s): VLESS expects *option.VLESSOutboundOptions, got %T", i, ob.Tag, ob.Options)
			}
		case C.TypeDirect:
			if ob.Options != nil {
				if _, ok := ob.Options.(*option.DirectOutboundOptions); !ok {
					return nil, fmt.Errorf("outbound %d (%s): Direct expects *option.DirectOutboundOptions or nil, got %T", i, ob.Tag, ob.Options)
				}
			}
		case C.TypeDNS, C.TypeBlock:
			if ob.Options != nil {
				if _, ok := ob.Options.(*option.StubOptions); !ok {
					return nil, fmt.Errorf("outbound %d (%s): %s expects *option.StubOptions or nil, got %T", i, ob.Tag, ob.Type, ob.Options)
				}
			}
		}
	}

	inst, err := box.New(box.Options{
		Context: ctx,
		Options: opts,
	})
	if err != nil {
		cancel()
		return nil, E.Cause(err, "create service")
	}

	// Если нужен хук режимов Clash — можно достать сервер из контекста
	var clash *clashapi.Server
	if opts.Experimental != nil && opts.Experimental.ClashAPI != nil {
		// Clash действительно включён в финальном конфиге — теперь можно брать из контекста
		clash = service.FromContext[*clashapi.Server](ctx) // может быть nil, это ок
	}

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

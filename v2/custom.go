package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Darkmen203/rostovvpn-core/bridge"
	"github.com/Darkmen203/rostovvpn-core/config"
	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/log"

	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

var (
	Box              *CoreService
	RostovVPNOptions *config.RostovVPNOptions
	activeConfigPath string
	coreLogFactory   log.Factory
	useFlutterBridge bool = true
	oldCommandServer *CommandServer
)

func startCommandServer() error {
	// размер буфера логов нам не нужен — оставим заглушку
	oldCommandServer = NewCommandServer(2048)
	return oldCommandServer.Start()
}

func StopAndAlert(msgType pb.MessageType, message string) {
	SetCoreStatus(pb.CoreState_STOPPED, msgType, message)
	config.DeactivateTunnelService()
	if oldCommandServer != nil {
		oldCommandServer.SetService(nil)
	}
	if Box != nil {
		Box.Close()
		Box = nil
	}
	if oldCommandServer != nil {
		oldCommandServer.Close()
	}
	if useFlutterBridge {
		alert := msgType.String()
		msg, _ := json.Marshal(StatusMessage{Status: convert2OldState(CoreState), Alert: &alert, Message: &message})
		bridge.SendStringToPort(statusPropagationPort, string(msg))
	}
}

func (s *CoreService) Start(ctx context.Context, in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	return Start(in)
}

func Start(in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	defer config.DeferPanicToError("start", func(err error) {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
	})
	Log(pb.LogLevel_INFO, pb.LogType_CORE, "Starting")
	if CoreState != pb.CoreState_STOPPED {
		Log(pb.LogLevel_INFO, pb.LogType_CORE, "Starting0000")
		Stop()
		// return &pb.CoreInfoResponse{
		// 	CoreState:   CoreState,
		// 	MessageType: pb.MessageType_INSTANCE_NOT_STOPPED,
		// }, fmt.Errorf("instance not stopped")
	}
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Starting Core")
	SetCoreStatus(pb.CoreState_STARTING, pb.MessageType_EMPTY, "")
	libbox.SetMemoryLimit(!in.DisableMemoryLimit)
	resp, err := StartService(in)
	return resp, err
}

func (s *CoreService) StartService(ctx context.Context, in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	return StartService(in)
}

func StartService(in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Starting Core Service")
	content := in.ConfigContent
	if content == "" {

		activeConfigPath = in.ConfigPath
		fileContent, err := os.ReadFile(activeConfigPath)
		if err != nil {
			Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
			resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_ERROR_READING_CONFIG, err.Error())
			StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
			return resp, err
		}
		content = string(fileContent)
	}
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Parsing Config")

	parsedContent, err := parseOptionsStrict(content)
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Parsed")

	if err != nil {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_ERROR_PARSING_CONFIG, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
		return resp, err
	}
	if !in.EnableRawConfig {
		Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Building config")
		if RostovVPNOptions == nil {
			RostovVPNOptions = config.DefaultRostovVPNOptions()
		}
		parsedContent_tmp, err := config.BuildConfig(*RostovVPNOptions, parsedContent)
		if err != nil {
			Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
			resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_ERROR_BUILDING_CONFIG, err.Error())
			StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
			return resp, err
		}

		// raw, err := json.Marshal(parsedContent_tmp)
		// if err != nil {
		// 	resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_ERROR_BUILDING_CONFIG, err.Error())

		// 	return resp, err
		// }

		// parsed, err := config.ParseConfigContent(string(raw), true, RostovVPNOptions, false)
		// if err != nil {
		// 	return parsed, fmt.Errorf("failed to reparse config via registry JSON: %w", err)
		// }

		parsedContent = *parsedContent_tmp
	}
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Saving config")
	currentBuildConfigPath := filepath.Join(sWorkingPath, "current-config.json")
	config.SaveCurrentConfig(currentBuildConfigPath, parsedContent)
	if activeConfigPath == "" {
		activeConfigPath = currentBuildConfigPath
	}
	if in.EnableOldCommandServer {
		Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Starting Command Server")
		err = startCommandServer()
		if err != nil {
			Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
			resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_START_COMMAND_SERVER, err.Error())
			StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
			return resp, err
		}
	}

	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Stating Service ")
	instance, err := NewService(parsedContent)
	if err != nil {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_CREATE_SERVICE, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
		return resp, err
	}
	Log(pb.LogLevel_DEBUG, pb.LogType_CORE, "Service.. started")
	if in.DelayStart {
		<-time.After(250 * time.Millisecond)
	}

	if err = instance.Run(); err != nil {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_START_SERVICE, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
		return resp, err
	}
	Box = instance
	if in.EnableOldCommandServer {
		oldCommandServer.SetService(Box) // если тут жёсткий тип *libbox.BoxService — см. примечание ниже
	}

	resp := SetCoreStatus(pb.CoreState_STARTED, pb.MessageType_EMPTY, "")
	return resp, nil
}

func (s *CoreService) Parse(ctx context.Context, in *pb.ParseRequest) (*pb.ParseResponse, error) {
	return Parse(in)
}

func Parse(in *pb.ParseRequest) (*pb.ParseResponse, error) {
	defer config.DeferPanicToError("parse", func(err error) {
		Log(pb.LogLevel_FATAL, pb.LogType_CONFIG, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
	})

	content := in.Content
	if in.TempPath != "" {
		contentBytes, err := os.ReadFile(in.TempPath)
		content = string(contentBytes)
		os.Chdir(filepath.Dir(in.ConfigPath))
		if err != nil {
			return nil, err
		}

	}

	config, err := config.ParseConfigContent(content, true, RostovVPNOptions, false)
	if err != nil {
		return &pb.ParseResponse{
			ResponseCode: pb.ResponseCode_FAILED,
			Message:      err.Error(),
		}, err
	}
	if in.ConfigPath != "" {
		err = os.WriteFile(in.ConfigPath, config, 0o644)
		if err != nil {
			return &pb.ParseResponse{
				ResponseCode: pb.ResponseCode_FAILED,
				Message:      err.Error(),
			}, err
		}
	}
	return &pb.ParseResponse{
		ResponseCode: pb.ResponseCode_OK,
		Content:      string(config),
		Message:      "",
	}, err
}

func (s *CoreService) ChangeRostovVPNSettings(ctx context.Context, in *pb.ChangeRostovVPNSettingsRequest) (*pb.CoreInfoResponse, error) {
	return ChangeRostovVPNSettings(in)
}

func ChangeRostovVPNSettings(in *pb.ChangeRostovVPNSettingsRequest) (*pb.CoreInfoResponse, error) {
	RostovVPNOptions = config.DefaultRostovVPNOptions()
	err := json.Unmarshal([]byte(in.GetRostovvpnSettingsJson()), RostovVPNOptions)
	if err != nil {
		return nil, err
	}
	return &pb.CoreInfoResponse{}, nil
}

func (s *CoreService) GenerateConfig(ctx context.Context, in *pb.GenerateConfigRequest) (*pb.GenerateConfigResponse, error) {
	return GenerateConfig(in)
}

func GenerateConfig(in *pb.GenerateConfigRequest) (*pb.GenerateConfigResponse, error) {
	defer config.DeferPanicToError("generateConfig", func(err error) {
		Log(pb.LogLevel_FATAL, pb.LogType_CONFIG, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
	})
	if RostovVPNOptions == nil {
		RostovVPNOptions = config.DefaultRostovVPNOptions()
	}
	config, err := generateConfigFromFile(in.Path, *RostovVPNOptions)
	if err != nil {
		return nil, err
	}
	return &pb.GenerateConfigResponse{
		ConfigContent: config,
	}, nil
}

// parseOptionsStrict парсит конфиг sing-box в строго типизированный option.Options.
func parseOptionsStrict(content string) (option.Options, error) {
	ctx := libbox.BaseContext(nil)
	// UnmarshalExtendedContext возвращает (T, error)
	return singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(content))
}

func generateConfigFromFile(path string, configOpt config.RostovVPNOptions) (string, error) {
	os.Chdir(filepath.Dir(path))
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	options, err := parseOptionsStrict(string(content))
	if err != nil {
		return "", err
	}
	config, err := config.BuildConfigJson(configOpt, options)
	if err != nil {
		return "", err
	}
	return config, nil
}

func (s *CoreService) Stop(ctx context.Context, empty *pb.Empty) (*pb.CoreInfoResponse, error) {
	return Stop()
}

func Stop() (*pb.CoreInfoResponse, error) {
	defer config.DeferPanicToError("stop", func(err error) {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
	})

	if CoreState != pb.CoreState_STARTED {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, "Core is not started")
		return &pb.CoreInfoResponse{
			CoreState:   CoreState,
			MessageType: pb.MessageType_INSTANCE_NOT_STARTED,
			Message:     "instance is not started",
		}, fmt.Errorf("instance not started")
	}
	if Box == nil {
		return &pb.CoreInfoResponse{
			CoreState:   CoreState,
			MessageType: pb.MessageType_INSTANCE_NOT_FOUND,
			Message:     "instance is not found",
		}, fmt.Errorf("instance not found")
	}
	SetCoreStatus(pb.CoreState_STOPPING, pb.MessageType_EMPTY, "")
	config.DeactivateTunnelService()
	if oldCommandServer != nil {
		oldCommandServer.SetService(nil)
	}

	err := Box.Close()
	if err != nil {
		return &pb.CoreInfoResponse{
			CoreState:   CoreState,
			MessageType: pb.MessageType_UNEXPECTED_ERROR,
			Message:     "Error while stopping the service.",
		}, fmt.Errorf("Error while stopping the service.")
	}
	Box = nil
	if oldCommandServer != nil {
		err = oldCommandServer.Close()
		if err != nil {
			return &pb.CoreInfoResponse{
				CoreState:   CoreState,
				MessageType: pb.MessageType_UNEXPECTED_ERROR,
				Message:     "Error while Closing the comand server.",
			}, fmt.Errorf("error while Closing the comand server.")
		}
		oldCommandServer = nil
	}
	resp := SetCoreStatus(pb.CoreState_STOPPED, pb.MessageType_EMPTY, "")
	return resp, nil
}

func (s *CoreService) Restart(ctx context.Context, in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	return Restart(in)
}

func Restart(in *pb.StartRequest) (*pb.CoreInfoResponse, error) {
	defer config.DeferPanicToError("restart", func(err error) {
		Log(pb.LogLevel_FATAL, pb.LogType_CORE, err.Error())
		StopAndAlert(pb.MessageType_UNEXPECTED_ERROR, err.Error())
	})
	log.Debug("[Service] Restarting")

	if CoreState != pb.CoreState_STARTED {
		return &pb.CoreInfoResponse{
			CoreState:   CoreState,
			MessageType: pb.MessageType_INSTANCE_NOT_STARTED,
			Message:     "instance is not started",
		}, fmt.Errorf("instance not started")
	}
	if Box == nil {
		return &pb.CoreInfoResponse{
			CoreState:   CoreState,
			MessageType: pb.MessageType_INSTANCE_NOT_FOUND,
			Message:     "instance is not found",
		}, fmt.Errorf("instance not found")
	}

	resp, err := Stop()
	if err != nil {
		return resp, err
	}

	SetCoreStatus(pb.CoreState_STARTING, pb.MessageType_EMPTY, "")
	<-time.After(250 * time.Millisecond)

	libbox.SetMemoryLimit(!in.DisableMemoryLimit)
	resp, gErr := StartService(in)
	return resp, gErr
}

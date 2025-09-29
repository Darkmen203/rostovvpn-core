// v2/command_client.go
package v2

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Darkmen203/rostovvpn-core/bridge"
	pb "github.com/Darkmen203/rostovvpn-core/rostovvpnrpc"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/log"
)

type ffiHandler struct {
	port   int64
	logger log.Logger
}

type groupItemJSON struct {
	Tag          string `json:"tag"`
	Type         string `json:"type"`
	URLTestTime  int64  `json:"url-test-time"`
	URLTestDelay int32  `json:"url-test-delay"`
}

type groupJSON struct {
	Tag      string          `json:"tag"`
	Type     string          `json:"type"`
	Selected string          `json:"selected"`
	Items    []groupItemJSON `json:"items"`
}

// Подтягиваем «эталонные» значения из текущего libbox
const (
	cmdLog                   = int32(libbox.CommandLog)
	cmdStatus                = int32(libbox.CommandStatus)
	cmdServiceReload         = int32(libbox.CommandServiceReload)
	cmdServiceClose          = int32(libbox.CommandServiceClose)
	cmdCloseConnections      = int32(libbox.CommandCloseConnections)
	cmdGroup                 = int32(libbox.CommandGroup)
	cmdSelectOutbound        = int32(libbox.CommandSelectOutbound)
	cmdURLTest               = int32(libbox.CommandURLTest)
	cmdGroupExpand           = int32(libbox.CommandGroupExpand)
	cmdClashMode             = int32(libbox.CommandClashMode)
	cmdSetClashMode          = int32(libbox.CommandSetClashMode)
	cmdGetSystemProxyStatus  = int32(libbox.CommandGetSystemProxyStatus)
	cmdSetSystemProxyEnabled = int32(libbox.CommandSetSystemProxyEnabled)
	cmdConnections           = int32(libbox.CommandConnections)
	cmdCloseConnection       = int32(libbox.CommandCloseConnection)
	cmdGetDeprecatedNotes    = int32(libbox.CommandGetDeprecatedNotes)
)

// Если Flutter enum совпадает — функция вернёт как есть.
// Если нет — сведи к «эталонным» значениям libbox.
func mapLibboxCmd(in int32) int32 {
	// 1) если Flutter уже прислал ровно libbox-константу — возвращаем как есть
	switch in {
	case cmdStatus:
		return cmdStatus
	case cmdGroup:
		return cmdGroup
	case cmdGroupExpand:
		return cmdGroupExpand
	case cmdLog:
		return cmdLog
	case cmdURLTest:
		return cmdURLTest
	}

	// 2) fallback для "своих" числовых enum-значений Flutter
	switch in {
	case 0:
		return cmdLog
	case 1:
		return cmdStatus
	case 2:
		return cmdServiceReload
	case 3:
		return cmdServiceClose
	case 4:
		return cmdCloseConnections
	case 5:
		return cmdGroup
	case 6:
		return cmdSelectOutbound
	case 7:
		return cmdURLTest
	case 8:
		return cmdGroupExpand
	case 9:
		return cmdClashMode
	case 10:
		return cmdSetClashMode
	case 11:
		return cmdGetSystemProxyStatus
	case 12:
		return cmdSetSystemProxyEnabled
	case 13:
		return cmdConnections
	case 14:
		return cmdCloseConnection
	case 15:
		return cmdGetDeprecatedNotes
	default:
		return cmdStatus
	}

}

var printedCmds bool

func printLibboxCmdsOnce() {
	if printedCmds {
		return
	}
	printedCmds = true
	fmt.Println("[FFI] libbox cmd ids:",
		" status=", int32(libbox.CommandStatus),
		" group=", int32(libbox.CommandGroup),
		" groupExpand=", int32(libbox.CommandGroupExpand),
		" log=", int32(libbox.CommandLog),
		" urlTest=", int32(libbox.CommandURLTest),
	)
}

func newFFIHandler(port int64, logger log.Logger) *ffiHandler {
	return &ffiHandler{port: port, logger: logger}
}

func (h *ffiHandler) Connected() { h.logger.Debug("[cmd] CONNECTED") }
func (h *ffiHandler) Disconnected(message string) {
	h.logger.Debug("[cmd] DISCONNECTED: ", message)
}
func (h *ffiHandler) ClearLogs() { /* no-op to UI */ }
func (h *ffiHandler) InitializeClashMode(_ libbox.StringIterator, cur string) {
	h.logger.Debug("[cmd] clash mode: ", cur)
}
func (h *ffiHandler) UpdateClashMode(newMode string) { h.logger.Debug("[cmd] clash mode -> ", newMode) }

// Логи: просто построчно прокидываем в Flutter
func (h *ffiHandler) WriteLogs(it libbox.StringIterator) {
	for it != nil && it.HasNext() {
		bridge.SendStringToPort(h.port, it.Next())
	}
}

// Статус: маршалим libbox.StatusMessage в JSON и шлём строкой
func (h *ffiHandler) WriteStatus(m *libbox.StatusMessage) {
	if m == nil {
		return
	}
	b, _ := json.Marshal(m)
	bridge.SendStringToPort(h.port, string(b))
	// параллельно можно подсветить в системный поток:
	systemInfoObserver.Emit(pb.SystemInfo{
		ConnectionsIn:  m.ConnectionsIn,
		ConnectionsOut: m.ConnectionsOut,
		Uplink:         m.Uplink,
		Downlink:       m.Downlink,
		UplinkTotal:    m.UplinkTotal,
		DownlinkTotal:  m.DownlinkTotal,
		Memory:         m.Memory,
		Goroutines:     m.Goroutines,
	})
}

// Группы: собираем «плоский» JSON, который удобно парсить во Flutter (SingboxOutboundGroup)
func (h *ffiHandler) WriteGroups(it libbox.OutboundGroupIterator) {
	var groups []groupJSON
	for it != nil && it.HasNext() {
		g := it.Next()
		var items []groupItemJSON
		i := g.GetItems()
		for i != nil && i.HasNext() {
			o := i.Next()
			items = append(items, groupItemJSON{
				Tag:          o.Tag,
				Type:         o.Type,
				URLTestTime:  o.URLTestTime,
				URLTestDelay: o.URLTestDelay,
			})
		}
		groups = append(groups, groupJSON{
			Tag:      g.Tag,
			Type:     g.Type,
			Selected: g.Selected,
			Items:    items,
		})
	}
	if len(groups) == 0 {
		return
	}
	b, _ := json.Marshal(groups)
	bridge.SendStringToPort(h.port, string(b))

	outboundsInfoObserver.Emit(toProtoGroups(groups))
	mainOutboundsInfoObserver.Emit(toProtoGroups(groups))
}

func toProtoGroups(gs []groupJSON) pb.OutboundGroupList {
	var out pb.OutboundGroupList
	for _, g := range gs {
		var items []*pb.OutboundGroupItem
		for _, it := range g.Items {
			items = append(items, &pb.OutboundGroupItem{
				Tag:          it.Tag,
				Type:         it.Type,
				UrlTestTime:  it.URLTestTime,
				UrlTestDelay: it.URLTestDelay,
			})
		}
		out.Items = append(out.Items, &pb.OutboundGroup{
			Tag: g.Tag, Type: g.Type, Selected: g.Selected, Items: items,
		})
	}
	return out
}

func (h *ffiHandler) WriteConnections(c *libbox.Connections) {
	// Можно тоже слать JSON; если пока не нужно во Flutter — просто лог.
	if c == nil {
		return
	}
	h.logger.Debug("[cmd] connections update")
}

// === Управление жизнью FFI-клиентов ===

var (
	ffiClientsMu sync.Mutex
	ffiClients   = map[int32]*libbox.CommandClient{}
)

// StartCommand: cmd — это КОД команды libbox (status/group/groupExpand/...), port — это DART SendPort.nativePort!
// Здесь НЕ трогаем TCP-порт — коннект берёт на себя libbox.CommandClient внутри sing-box.
func StartCommand(cmd int32, dartPort int64) error {
	printLibboxCmdsOnce()
	coreLogFactory.NewLogger("[FFI]").Info(
		fmt.Sprintf("StartCommand: cmd=%d dartPort=%d", cmd, dartPort),
	)

	ffiClientsMu.Lock()
	if old := ffiClients[cmd]; old != nil {
		old.Disconnect()
	}
	ffiClientsMu.Unlock()

	logger := coreLogFactory.NewLogger(fmt.Sprintf("[FFI Command %d]", cmd))
	handler := newFFIHandler(dartPort, logger)

	mapped := mapLibboxCmd(cmd)
	opts := &libbox.CommandClientOptions{
		Command:        mapped,
		StatusInterval: int64(time.Second),
	}
	client := libbox.NewCommandClient(handler, opts)
	logger.Info("connecting to CommandServer ...")
	if err := client.Connect(); err != nil {
		logger.Error("connect failed: ", err)
		return err
	}
	logger.Info("connect ok")

	ffiClientsMu.Lock()
	ffiClients[cmd] = client
	ffiClientsMu.Unlock()
	return nil
}

func StopCommand(cmd int32) error {
	ffiClientsMu.Lock()
	c := ffiClients[cmd]
	delete(ffiClients, cmd)
	ffiClientsMu.Unlock()
	if c != nil {
		c.Disconnect()
	}
	return nil
}

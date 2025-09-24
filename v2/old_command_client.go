//go:build legacy
package v2

import (
	"encoding/json"
	"fmt"

	"github.com/Darkmen203/rostovvpn-core/bridge"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/log"
)

var (
	_ libbox.CommandClientHandler = (*OldCommandClientHandler)(nil)
)

type OldCommandClientHandler struct {
	port   int64
	logger log.Logger
}

func (cch *OldCommandClientHandler) Connected() {
	cch.logger.Debug("CONNECTED")
}

func (cch *OldCommandClientHandler) Disconnected(message string) {
	cch.logger.Debug("DISCONNECTED: ", message)
}

func (cch *OldCommandClientHandler) ClearLogs() {
	cch.logger.Debug("clear log")
}

func (cch *OldCommandClientHandler) WriteLogs(messageList libbox.StringIterator) {
	for messageList != nil && messageList.HasNext() {
		message := messageList.Next()
		cch.logger.Debug("log: ", message)
		bridge.SendStringToPort(cch.port, message)
	}
}

func (cch *OldCommandClientHandler) WriteStatus(message *libbox.StatusMessage) {
	msg, err := json.Marshal(
		map[string]int64{
			"connections-in":  int64(message.ConnectionsIn),
			"connections-out": int64(message.ConnectionsOut),
			"uplink":          message.Uplink,
			"downlink":        message.Downlink,
			"uplink-total":    message.UplinkTotal,
			"downlink-total":  message.DownlinkTotal,
		},
	)
	cch.logger.Debug("Memory: ", libbox.FormatBytes(message.Memory), ", Goroutines: ", message.Goroutines)
	if err != nil {
		bridge.SendStringToPort(cch.port, fmt.Sprintf("error: %e", err))
	} else {
		bridge.SendStringToPort(cch.port, string(msg))
	}
}

func (cch *OldCommandClientHandler) WriteGroups(message libbox.OutboundGroupIterator) {
	if message == nil {
		return
	}
	groups := []*OutboundGroup{}
	for message.HasNext() {
		group := message.Next()
		items := group.GetItems()
		groupItems := []*OutboundGroupItem{}
		for items.HasNext() {
			item := items.Next()
			groupItems = append(groupItems,
				&OutboundGroupItem{
					Tag:          item.Tag,
					Type:         item.Type,
					URLTestTime:  item.URLTestTime,
					URLTestDelay: item.URLTestDelay,
				},
			)
		}
		groups = append(groups, &OutboundGroup{Tag: group.Tag, Type: group.Type, Selected: group.Selected, Items: groupItems})
	}
	response, err := json.Marshal(groups)
	if err != nil {
		bridge.SendStringToPort(cch.port, fmt.Sprintf("error: %e", err))
	} else {
		bridge.SendStringToPort(cch.port, string(response))
	}
}

func (cch *OldCommandClientHandler) InitializeClashMode(modeList libbox.StringIterator, currentMode string) {
	cch.logger.Debug("initial clash mode: ", currentMode)
}

func (cch *OldCommandClientHandler) UpdateClashMode(newMode string) {
	cch.logger.Debug("update clash mode: ", newMode)
}

func (cch *OldCommandClientHandler) WriteConnections(message *libbox.Connections) {
	if message == nil {
		return
	}
	message.FilterState(libbox.ConnectionStateAll)
	iter := message.Iterator()
	type connectionInfo struct {
		ID            string `json:"id"`
		Inbound       string `json:"inbound"`
		Outbound      string `json:"outbound"`
		Network       string `json:"network"`
		Source        string `json:"source"`
		Destination   string `json:"destination"`
		Protocol      string `json:"protocol"`
		User          string `json:"user"`
		CreatedAt     int64  `json:"created_at"`
		ClosedAt      int64  `json:"closed_at"`
		Uplink        int64  `json:"uplink"`
		Downlink      int64  `json:"downlink"`
		UplinkTotal   int64  `json:"uplink_total"`
		DownlinkTotal int64  `json:"downlink_total"`
	}
	connections := make([]connectionInfo, 0)
	for iter != nil && iter.HasNext() {
		conn := iter.Next()
		if conn == nil {
			continue
		}
		connections = append(connections, connectionInfo{
			ID:            conn.ID,
			Inbound:       conn.Inbound,
			Outbound:      conn.Outbound,
			Network:       conn.Network,
			Source:        conn.Source,
			Destination:   conn.Destination,
			Protocol:      conn.Protocol,
			User:          conn.User,
			CreatedAt:     conn.CreatedAt,
			ClosedAt:      conn.ClosedAt,
			Uplink:        conn.Uplink,
			Downlink:      conn.Downlink,
			UplinkTotal:   conn.UplinkTotal,
			DownlinkTotal: conn.DownlinkTotal,
		})
	}
	if len(connections) == 0 {
		return
	}
	payload, err := json.Marshal(connections)
	if err != nil {
		cch.logger.Error("marshal connections: ", err)
		return
	}
	bridge.SendStringToPort(cch.port, string(payload))
}

type OutboundGroup struct {
	Tag      string               `json:"tag"`
	Type     string               `json:"type"`
	Selected string               `json:"selected"`
	Items    []*OutboundGroupItem `json:"items"`
}

type OutboundGroupItem struct {
	Tag          string `json:"tag"`
	Type         string `json:"type"`
	URLTestTime  int64  `json:"url-test-time"`
	URLTestDelay int32  `json:"url-test-delay"`
}

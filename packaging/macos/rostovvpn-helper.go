//go:build darwin

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
)

type req struct {
	Cmd     string         `json:"cmd"` // start_tun|stop_tun
	Options map[string]any `json:"options"`
}
type resp struct {
	Ok  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
}

func rpc(socket string, handler func(r req) resp) error {
	_ = os.Remove(socket)
	l, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go func(conn net.Conn) {
			defer conn.Close()
			dec := json.NewDecoder(bufio.NewReader(conn))
			var r req
			if err := dec.Decode(&r); err != nil {
				json.NewEncoder(conn).Encode(resp{Ok: false, Err: err.Error()})
				return
			}
			ans := handler(r)
			json.NewEncoder(conn).Encode(ans)
		}(c)
	}
}

func main() {
	socket := "/var/run/rostovvpn.sock"
	if len(os.Args) >= 3 && os.Args[1] == "--socket" {
		socket = os.Args[2]
	}
	handler := func(r req) resp {
		switch r.Cmd {
		case "start_tun":
			cfg := "/Library/Application Support/RostovVPN/current-config.json"
			if v, ok := r.Options["config"].(string); ok && v != "" {
				cfg = v
			}
			// Запускаем ваш rvpncli в фоне
			cmd := exec.Command("/usr/local/bin/rvpncli", "--action", "start", "--enable-tun", "--mtu", "1450", "--config", cfg)
			if err := cmd.Start(); err != nil {
				return resp{Ok: false, Err: err.Error()}
			}
			return resp{Ok: true}
		case "stop_tun":
			cfg := "/Library/Application Support/RostovVPN/current-config.json"
			if v, ok := r.Options["config"].(string); ok && v != "" {
				cfg = v
			}
			cmd := exec.Command("/usr/local/bin/rvpncli", "--action", "stop", "--config", cfg)
			_ = cmd.Run()
			return resp{Ok: true}
		default:
			return resp{Ok: false, Err: "unknown cmd"}
		}
	}
	if err := rpc(socket, handler); err != nil {
		fmt.Fprintln(os.Stderr, "helper rpc error:", err)
		os.Exit(1)
	}
}

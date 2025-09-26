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
	Cmd     string         `json:"cmd"`
	Options map[string]any `json:"options"`
}
type resp struct {
	Ok  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
}

func serve(socket string) error {
	_ = os.Remove(socket)
	l, err := net.Listen("unix", socket)
	if err != nil { return err }
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil { return err }
		go func(conn net.Conn) {
			defer conn.Close()
			dec := json.NewDecoder(bufio.NewReader(conn))
			var r req
			if err := dec.Decode(&r); err != nil {
				_ = json.NewEncoder(conn).Encode(resp{Ok:false, Err:err.Error()})
				return
			}
			var out resp
			switch r.Cmd {
			case "start_tun":
				cfg := "/Library/Application Support/RostovVPN/current-config.json"
				if v, ok := r.Options["config"].(string); ok && v != "" { cfg = v }
				cmd := exec.Command("/usr/local/bin/rvpncli", "--action", "start", "--enable-tun", "--mtu", "1450", "--config", cfg)
				if err := cmd.Start(); err != nil {
					out = resp{Ok:false, Err:err.Error()}
				} else {
					out = resp{Ok:true}
				}
			case "stop_tun":
				cfg := "/Library/Application Support/RostovVPN/current-config.json"
				if v, ok := r.Options["config"].(string); ok && v != "" { cfg = v }
				cmd := exec.Command("/usr/local/bin/rvpncli", "--action", "stop", "--config", cfg)
				_ = cmd.Run()
				out = resp{Ok:true}
			default:
				out = resp{Ok:false, Err:"unknown cmd"}
			}
			_ = json.NewEncoder(conn).Encode(out)
		}(c)
	}
}

func main() {
	socket := "/var/run/rostovvpn.sock"
	if len(os.Args) >= 3 && os.Args[1] == "--socket" {
		socket = os.Args[2]
	}
	if err := serve(socket); err != nil {
		fmt.Fprintln(os.Stderr, "helper error:", err)
		os.Exit(1)
	}
}

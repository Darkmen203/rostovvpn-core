//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func exeDir() string {
	p, _ := os.Executable()
	return filepath.Dir(p)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: rostovvpn-helper --start-tun|--stop-tun --config <path>")
		os.Exit(2)
	}
	dir := exeDir()
	cli := filepath.Join(dir, "rvpncli.exe")
	cfg := ""
	for i := 0; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			cfg = os.Args[i+1]
		}
	}
	if cfg == "" {
		// дефолт: туда, куда уже пишете current-config.json
		cfg = filepath.Join(os.Getenv("APPDATA"), "RostovVPN", "rostovvpn", "current-config.json")
	}

	switch os.Args[1] {
	case "--start-tun":
		cmd := exec.Command(cli, "--action", "start", "--enable-tun", "--mtu", "1450", "--config", cfg)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println("start tun failed:", err)
			os.Exit(1)
		}
	case "--stop-tun":
		cmd := exec.Command(cli, "--action", "stop", "--config", cfg)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		_ = cmd.Run()
	default:
		fmt.Println("unknown command:", os.Args[1])
		os.Exit(2)
	}
}

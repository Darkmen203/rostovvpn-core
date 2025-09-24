//go:build windows || darwin || linux

package v2

import (
	"os"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// Безопасная платформа для desktop: ничего не трогаем, TUN не открываем.
type platformStub struct{}

func (platformStub) LocalDNSTransport() libbox.LocalDNSTransport { return nil }

// КРИТИЧНО: отключаем автоконтроль интерфейса/сокетов
func (platformStub) UsePlatformAutoDetectInterfaceControl() bool { return false }
func (platformStub) AutoDetectInterfaceControl(fd int32) error   { return nil }

func (platformStub) OpenTun(_ libbox.TunOptions) (int32, error)  { return 0, os.ErrInvalid }
func (platformStub) WriteLog(string)                             {}
func (platformStub) UseProcFS() bool                             { return false }
func (platformStub) FindConnectionOwner(int32, string, int32, string, int32) (int32, error) {
	return -1, os.ErrInvalid
}
func (platformStub) PackageNameByUid(int32) (string, error)                  { return "", nil }
func (platformStub) UIDByPackageName(string) (int32, error)                  { return -1, os.ErrInvalid }
func (platformStub) StartDefaultInterfaceMonitor(libbox.InterfaceUpdateListener) error { return nil }
func (platformStub) CloseDefaultInterfaceMonitor(libbox.InterfaceUpdateListener) error { return nil }
func (platformStub) GetInterfaces() (libbox.NetworkInterfaceIterator, error) { return nil, nil }
func (platformStub) UnderNetworkExtension() bool                             { return false }
func (platformStub) IncludeAllNetworks() bool                                { return false }
func (platformStub) ReadWIFIState() *libbox.WIFIState                        { return nil }
func (platformStub) SystemCertificates() libbox.StringIterator               { return nil }
func (platformStub) ClearDNSCache()                                          {}
func (platformStub) SendNotification(*libbox.Notification) error             { return nil }

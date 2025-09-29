package v2

import (
	"fmt"

	"github.com/sagernet/sing-box/log"
)

func logInfof(format string, a ...any) { log.Info(fmt.Sprintf(format, a...)) }
func logWarnf(format string, a ...any) { log.Warn(fmt.Sprintf(format, a...)) }
func logErrf(format string, a ...any)  { log.Error(fmt.Sprintf(format, a...)) }
func logDbgf(format string, a ...any)  { log.Debug(fmt.Sprintf(format, a...)) }

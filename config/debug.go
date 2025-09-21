package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

//	func SaveCurrentConfig(path string, options option.Options) error {
//		cfg, err := ToJson(options)
//		if err != nil {
//			return err
//		}
//		p, err := filepath.Abs(path)
//		fmt.Printf("Saving config to %v %+v\n", p, err)
//		if err != nil {
//			return err
//		}
//		return os.WriteFile(p, []byte(cfg), 0644)
//	}
func SaveCurrentConfig(path string, opts option.Options) error {
	ctx := libbox.BaseContext(nil)

	// сериализация с учётом полиморфных полей sing-box
	b, err := singjson.MarshalContext(ctx, opts)
	if err != nil {
		return err
	}

	// (опционально) красиво отформатировать
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, b, "", "  "); err == nil {
		b = pretty.Bytes()
	}

	p, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fmt.Printf("Saving config to %v %v\n", p, err)
	return os.WriteFile(p, b, 0644)
}

func ToJson(opt option.Options) (string, error) {
	ctx := libbox.BaseContext(nil)

	// Правильный маршалинг polymorphic-полей sing-box
	b, err := singjson.MarshalContext(ctx, opt)
	if err != nil {
		// Отладка, если вдруг что-то не сериализуется
		fmt.Println("[ToJson] MarshalContext failed:", err)
		fmt.Println("[ToJson] Outbounds dump (type, tag, options type):")
		for i, ob := range opt.Outbounds {
			optType := "nil"
			if ob.Options != nil {
				optType = fmt.Sprintf("%T", ob.Options)
			}
			fmt.Printf("  [%d] type=%s tag=%s options=%s\n", i, ob.Type, ob.Tag, optType)
		}
		// Фоллбэк — обычный encoder, чтобы хотя бы увидеть JSON
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err2 := enc.Encode(opt); err2 == nil {
			return buf.String(), nil
		}
		return "", err
	}

	// Красиво отформатируем
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return string(b), nil
	}
	return out.String(), nil
}

func DeferPanicToError(name string, err func(error)) {
	if r := recover(); r != nil {
		s := fmt.Errorf("%s panic: %s\n%s", name, r, string(debug.Stack()))
		err(s)
	}
}

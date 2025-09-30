package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Darkmen203/ray2sing/ray2sing"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	SJ "github.com/sagernet/sing/common/json"
	"github.com/xmdhs/clash2singbox/convert"
	"github.com/xmdhs/clash2singbox/model/clash"
	"gopkg.in/yaml.v3"
)

//go:embed config.json.template
var configByte []byte

func ParseConfig(path string, debug bool) ([]byte, error) {
	content, err := os.ReadFile(path)
	os.Chdir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return ParseConfigContent(string(content), debug, nil, false)
}

func ParseConfigContentToOptions(contentstr string, debug bool, configOpt *RostovVPNOptions, fullConfig bool) (*option.Options, error) {
	content, err := ParseConfigContent(contentstr, debug, configOpt, fullConfig)
	if err != nil {
		return nil, err
	}
	var options option.Options
	err = SJ.Unmarshal(content, &options)
	if err != nil {
		return nil, err
	}
	return &options, nil
}

func ParseConfigContent(contentstr string, debug bool, configOpt *RostovVPNOptions, fullConfig bool) ([]byte, error) {
	// fmt.Print("\n[config.ParseConfigContent] !!! configOpt= \n", configOpt, ",\n  !!! [config.ParseConfigContent] \n")
	
	if configOpt == nil {
		configOpt = DefaultRostovVPNOptions()
	}
	content := []byte(contentstr)
	var jsonObj map[string]interface{} = make(map[string]interface{})

	// fmt.Printf("Convert using json\n")
	var tmpJsonResult any
	jsonDecoder := json.NewDecoder(SJ.NewCommentFilter(bytes.NewReader(content)))
	if err := jsonDecoder.Decode(&tmpJsonResult); err == nil {
		if tmpJsonObj, ok := tmpJsonResult.(map[string]interface{}); ok {
			if tmpJsonObj["outbounds"] == nil {
				jsonObj["outbounds"] = []interface{}{jsonObj}
			} else {
				if fullConfig || (configOpt != nil && configOpt.EnableFullConfig) {
					jsonObj = tmpJsonObj
				} else {
					jsonObj["outbounds"] = tmpJsonObj["outbounds"]
					if exp, ok := tmpJsonObj["experimental"]; ok {
						jsonObj["experimental"] = exp
					}
					if lg, ok := tmpJsonObj["log"]; ok {
						jsonObj["log"] = lg
					}
				}
			}
		} else if jsonArray, ok := tmpJsonResult.([]interface{}); ok {
			jsonObj["outbounds"] = jsonArray
		} else {
			return nil, fmt.Errorf("[SingboxParser] Incorrect Json Format")
		}

		newContent, _ := json.MarshalIndent(jsonObj, "", "  ")

		return patchConfig(newContent, "SingboxParser", configOpt)
	}

	v2rayStr, err := ray2sing.Ray2Singbox(string(content), configOpt.UseXrayCoreWhenPossible)
	if err == nil {
		return patchConfig([]byte(v2rayStr), "V2rayParser", configOpt)
	}
	// fmt.Printf("Convert using clash\n")
	clashObj := clash.Clash{}
	if err := yaml.Unmarshal(content, &clashObj); err == nil && clashObj.Proxies != nil {
		if len(clashObj.Proxies) == 0 {
			return nil, fmt.Errorf("[ClashParser] no outbounds found")
		}
		converted, err := convert.Clash2sing(clashObj)
		if err != nil {
			return nil, fmt.Errorf("[ClashParser] converting clash to sing-box error: %w", err)
		}
		output := configByte
		output, err = convert.Patch(output, converted, "", "", nil)
		if err != nil {
			return nil, fmt.Errorf("[ClashParser] patching clash config error: %w", err)
		}
		return patchConfig(output, "ClashParser", configOpt)
	}

	return nil, fmt.Errorf("unable to determine config format")
}

func patchConfig(content []byte, name string, configOpt *RostovVPNOptions) ([]byte, error) {
	// 1) Разбираем как обычную JSON-карту (без реестров/типов)
	var root any
	dec := json.NewDecoder(SJ.NewCommentFilter(bytes.NewReader(content)))
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("[SingboxParser] json decode error: %w", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("[SingboxParser] root must be JSON object")
	}
	// 2) Достаём outbounds и патчим каждый как map[string]any
	rawOuts, ok := obj["outbounds"].([]any)
	if !ok {
		// может быть отсутствует (тогда ничего патчить не надо)
		goto DUMP_AND_VALIDATE
	}
	for i := range rawOuts {
		m, ok := rawOuts[i].(map[string]any)
		if !ok {
			continue
		}
		// 3.1. Удаляем легаси-поле у direct-аута
		if t, _ := m["type"].(string); strings.EqualFold(t, "direct") {
			delete(m, "tls_fragment") // поле больше не поддерживается
			if tag, _ := m["tag"].(string); tag == "direct-fragment" {
				m["tag"] = "direct" // или вообще выкинуть этот аут
			}
		}

		// 3.2. Твой текущий патч (warp и т.д.)
		patched, err := patchWarpMap(outboundMap(m), configOpt, false, nil)
		if err != nil {
			return nil, fmt.Errorf("[Warp] patch warp error: %w", err)
		}
		rawOuts[i] = map[string]any(patched)
	}
	obj["outbounds"] = rawOuts

DUMP_AND_VALIDATE:
	content, _ = json.MarshalIndent(obj, "", "  ")
	// fmt.Printf("%s\n", content)
	return validateResult(content, name)
}

func validateResult(content []byte, name string) ([]byte, error) {
	err := libbox.CheckConfig(string(content))
	if err != nil {
		return nil, fmt.Errorf("[%s] invalid sing-box config: %w", name, err)
	}
	return content, nil
}

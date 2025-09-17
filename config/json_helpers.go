package config

import (
	"encoding/json"
	"fmt"

	option "github.com/sagernet/sing-box/option"
)

// outboundMap is a helper alias for working with JSON objects that represent
// sing-box outbound definitions.
type outboundMap map[string]any

func marshalToMap(value any) (outboundMap, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err = json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return outboundMap(obj), nil
}

func mapToStruct(obj outboundMap, target any) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (m outboundMap) clone() outboundMap {
	result := make(outboundMap, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func (m outboundMap) string(key string) string {
	if value, ok := m[key]; ok {
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}

func (m outboundMap) bool(key string) bool {
	if value, ok := m[key]; ok {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}

func (m outboundMap) float64(key string) (float64, bool) {
	if value, ok := m[key]; ok {
		if f, ok := value.(float64); ok {
			return f, true
		}
	}
	return 0, false
}

func (m outboundMap) nestedMap(key string) (outboundMap, bool) {
	value, ok := m[key]
	if !ok {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok {
		return outboundMap(typed), true
	}
	return nil, false
}

func (m outboundMap) ensureNestedMap(key string) outboundMap {
	nested, ok := m.nestedMap(key)
	if ok {
		return nested
	}
	created := make(map[string]any)
	m[key] = created
	return outboundMap(created)
}

func (m outboundMap) delete(keys ...string) {
	if len(keys) == 0 {
		return
	}
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			delete(current, key)
			return
		}
		next, ok := current.nestedMap(key)
		if !ok {
			return
		}
		current = next
	}
}

func (m outboundMap) get(keys ...string) (any, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	current := map[string]any(m)
	for i, key := range keys {
		value, ok := current[key]
		if !ok {
			return nil, false
		}
		if i == len(keys)-1 {
			return value, true
		}
		next, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func (m outboundMap) set(keys []string, value any) error {
	if len(keys) == 0 {
		return fmt.Errorf("empty key path")
	}
	current := map[string]any(m)
	for i, key := range keys {
		if i == len(keys)-1 {
			current[key] = value
			return nil
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[key] = next
		}
		current = next
	}
	return nil
}

func outboundToMap(out option.Outbound) (outboundMap, error) {
	return marshalToMap(out)
}

func mapToOutbound(obj outboundMap) (option.Outbound, error) {
	var out option.Outbound
	if err := mapToStruct(obj, &out); err != nil {
		return option.Outbound{}, err
	}
	return out, nil
}

func mapToDNSServer(obj outboundMap) (option.DNSServerOptions, error) {
	var server option.DNSServerOptions
	if err := mapToStruct(obj, &server); err != nil {
		return option.DNSServerOptions{}, err
	}
	return server, nil
}

func mapToInbound(obj outboundMap) (option.Inbound, error) {
	var inbound option.Inbound
	if err := mapToStruct(obj, &inbound); err != nil {
		return option.Inbound{}, err
	}
	return inbound, nil
}

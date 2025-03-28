package bidirpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

// decodeInto decodes msg.Result into the provided pointer result.
func decodeInto(result any, src any) error {
	if result == nil {
		return errors.New("result pointer is nil")
	}
	ptrVal := reflect.ValueOf(result)
	if ptrVal.Kind() != reflect.Ptr || ptrVal.IsNil() {
		return errors.New("result must be a non-nil pointer")
	}

	// If types match directly
	if reflect.TypeOf(src) == ptrVal.Elem().Type() {
		ptrVal.Elem().Set(reflect.ValueOf(src))
		return nil
	}

	// Try with mapstructure if src is map
	if m, ok := src.(map[string]any); ok {
		cfg := &mapstructure.DecoderConfig{
			TagName:          "json",
			Result:           result,
			WeaklyTypedInput: true,
		}
		decoder, err := mapstructure.NewDecoder(cfg)
		if err != nil {
			return err
		}
		return decoder.Decode(m)
	}

	// Fallback: marshal then unmarshal
	data, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("encode fallback: %w", err)
	}
	return json.Unmarshal(data, result)
}

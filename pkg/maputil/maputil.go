package maputil

import (
	"fmt"
)

func CastKeysToStrings(s any) (map[string]any, error) {
	new := map[string]any{}
	switch src := s.(type) {
	case map[any]any:
		for k, v := range src {
			var strK string
			switch typedK := k.(type) {
			case string:
				strK = typedK
			default:
				return nil, fmt.Errorf("unexpected type of key in map: expected string, got %T: value=%v, map=%v", typedK, typedK, src)
			}

			castedV, err := RecursivelyStringifyMapKey(v)
			if err != nil {
				return nil, err
			}

			new[strK] = castedV
		}
	case map[string]any:
		for k, v := range src {
			castedV, err := RecursivelyStringifyMapKey(v)
			if err != nil {
				return nil, err
			}

			new[k] = castedV
		}
	}
	return new, nil
}

func RecursivelyStringifyMapKey(v any) (any, error) {
	var castedV any
	switch typedV := v.(type) {
	case map[any]any, map[string]any:
		tmp, err := CastKeysToStrings(typedV)
		if err != nil {
			return nil, err
		}
		castedV = tmp
	case []any:
		a := []any{}
		for i := range typedV {
			res, err := RecursivelyStringifyMapKey(typedV[i])
			if err != nil {
				return nil, err
			}
			a = append(a, res)
		}
		castedV = a
	default:
		castedV = typedV
	}
	return castedV, nil
}

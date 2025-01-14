package redact

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/supergoodsystems/supergood-go/internal/shared"
	"github.com/supergoodsystems/supergood-go/pkg/event"
)

var shouldTraverseKind = map[reflect.Kind]struct{}{
	reflect.Uintptr:       {},
	reflect.Array:         {},
	reflect.Interface:     {},
	reflect.Map:           {},
	reflect.Pointer:       {},
	reflect.Slice:         {},
	reflect.Struct:        {},
	reflect.UnsafePointer: {},
}

func redactAll(domain, url string, e *event.Event) ([]event.RedactedKeyMeta, error) {
	meta := []event.RedactedKeyMeta{}
	redactRequestBodyMeta, err := redactAllHelper(reflect.ValueOf(e.Request.Body), shared.RequestBodyStr)
	if err != nil {
		return []event.RedactedKeyMeta{}, err
	}
	meta = append(meta, redactRequestBodyMeta...)

	redactRequestHeaderMeta, err := redactAllHelper(reflect.ValueOf(e.Request.Headers), shared.RequestHeadersStr)
	if err != nil {
		return []event.RedactedKeyMeta{}, err
	}
	meta = append(meta, redactRequestHeaderMeta...)

	redactResponseBodyMeta, err := redactAllHelper(reflect.ValueOf(e.Response.Body), shared.ResponseBodyStr)
	if err != nil {
		return []event.RedactedKeyMeta{}, err
	}
	meta = append(meta, redactResponseBodyMeta...)

	redactResponseHeaderMeta, err := redactAllHelper(reflect.ValueOf(e.Response.Headers), shared.ResponseHeadersStr)
	if err != nil {
		return []event.RedactedKeyMeta{}, err
	}
	meta = append(meta, redactResponseHeaderMeta...)

	return meta, nil
}

func redactAllHelper(v reflect.Value, path string) ([]event.RedactedKeyMeta, error) {
	if !v.IsValid() {
		return prepareNilOutput(path), nil
	}
	switch v.Type().Kind() {
	case reflect.Ptr, reflect.Interface:
		return redactAllHelper(v.Elem(), path)

	case reflect.Map:
		results := []event.RedactedKeyMeta{}
		for _, e := range v.MapKeys() {
			mapVal := v.MapIndex(e)
			path := path + "." + e.String()
			if !mapVal.IsValid() {
				return prepareNilOutput(path), nil
			}
			if mapVal.Kind() == reflect.Interface || mapVal.Kind() == reflect.Pointer {
				mapVal = mapVal.Elem()
			}
			// checl again after returning the underlying value
			if !mapVal.IsValid() {
				return prepareNilOutput(path), nil
			}

			ok := shouldTraverse(mapVal)
			if !ok {
				v.SetMapIndex(e, reflect.Zero(mapVal.Type()))
				results = append(results, prepareOutput(mapVal, path)...)
			} else {
				result, err := redactAllHelper(mapVal, path)
				if err != nil {
					return results, err
				}
				results = append(results, result...)
			}

		}
		return results, nil

	case reflect.Array, reflect.Slice:
		results := []event.RedactedKeyMeta{}
		for i := 0; i < v.Len(); i++ {
			result, err := redactAllHelper(v.Index(i), path+"["+strconv.Itoa(i)+"]")
			if err != nil {
				return results, err
			}
			results = append(results, result...)
		}
		return results, nil

	default:
		// NOTE: We pass in directly to this func
		// request.Body, response.Body, request.Headers, response.headers
		// we can expect the form of these to be of map[string]any or a byte array
		return nil, fmt.Errorf("unexpected event structure")
	}
}

func shouldTraverse(v reflect.Value) bool {
	switch v.Kind() {
	// NOTE: below is required to redact arrays and slices.
	// Arrays, slices with primitive values cannot be successfully nullified because
	// the reflected value is not addressable
	case reflect.Array, reflect.Slice:
		if v.Len() == 0 {
			return false
		}
		valAtIndex := v.Index(0)
		k := valAtIndex.Kind()
		if k == reflect.Interface || k == reflect.Pointer {
			valAtIndex = valAtIndex.Elem()
		}
		_, ok := shouldTraverseKind[valAtIndex.Kind()]
		return ok

	case reflect.Uintptr, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Struct, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

func prepareNilOutput(path string) []event.RedactedKeyMeta {
	return []event.RedactedKeyMeta{
		{
			KeyPath: path,
			Length:  0,
			Type:    "invalid",
		},
	}
}

func prepareOutput(v reflect.Value, path string) []event.RedactedKeyMeta {
	size := getSize(v)
	return []event.RedactedKeyMeta{
		{
			KeyPath: path,
			Length:  size,
			Type:    formatKind(v.Type().Kind()),
		},
	}
}

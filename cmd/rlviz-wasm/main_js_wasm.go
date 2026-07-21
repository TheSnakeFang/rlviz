//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"syscall/js"

	"github.com/TheSnakeFang/rlviz/internal/browsercore"
)

const maxCachedCollections = 4

var (
	callbacks       []js.Func
	collectionCache = map[string]browsercore.Collection{}
	collectionOrder []string
)

type bridgeResult struct {
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type codedError interface {
	ErrorCode() string
}

func expose(name string, callback func([]js.Value) (any, error)) {
	fn := js.FuncOf(func(_ js.Value, args []js.Value) (encoded any) {
		deferred := bridgeResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				deferred = bridgeResult{OK: false, Error: fmt.Sprintf("viewer core panic: %v", recovered)}
			}
			payload, err := json.Marshal(deferred)
			if err != nil {
				encoded = `{"ok":false,"error":"could not encode result"}`
				return
			}
			encoded = string(payload)
		}()
		value, err := callback(args)
		if err != nil {
			deferred = bridgeResult{OK: false, Error: err.Error()}
			var coded codedError
			if errors.As(err, &coded) {
				deferred.Code = coded.ErrorCode()
			}
		} else {
			deferred = bridgeResult{OK: true, Value: value}
		}
		return nil
	})
	callbacks = append(callbacks, fn)
	js.Global().Set(name, fn)
}

func bytesArgument(value js.Value, label string) ([]byte, error) {
	uint8Array := js.Global().Get("Uint8Array")
	if value.Type() != js.TypeObject || !value.InstanceOf(uint8Array) {
		return nil, fmt.Errorf("%s must be a Uint8Array", label)
	}
	length := value.Get("byteLength")
	if length.Type() != js.TypeNumber || length.Int() < 0 {
		return nil, fmt.Errorf("%s has an invalid byteLength", label)
	}
	data := make([]byte, length.Int())
	if copied := js.CopyBytesToGo(data, value); copied != len(data) {
		return nil, fmt.Errorf("could not copy complete %s", label)
	}
	return data, nil
}

func stringArgument(value js.Value, label string) (string, error) {
	if value.Type() != js.TypeString {
		return "", fmt.Errorf("%s must be a string", label)
	}
	return value.String(), nil
}

func cacheCollection(collection browsercore.Collection) (map[string]any, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	id := hex.EncodeToString(sum[:])
	if _, exists := collectionCache[id]; !exists {
		collectionCache[id] = collection
		collectionOrder = append(collectionOrder, id)
		if len(collectionOrder) > maxCachedCollections {
			delete(collectionCache, collectionOrder[0])
			collectionOrder = collectionOrder[1:]
		}
	}
	return map[string]any{"collection_id": id, "collection": json.RawMessage(encoded)}, nil
}

func cachedCollection(value js.Value) (browsercore.Collection, error) {
	id, err := stringArgument(value, "collection digest")
	if err != nil {
		return browsercore.Collection{}, err
	}
	collection, ok := collectionCache[id]
	if !ok {
		return browsercore.Collection{}, errors.New("browser collection is no longer cached; reopen the trace")
	}
	return collection, nil
}

func main() {
	expose("rlvizParse", func(args []js.Value) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("rlvizParse expects Uint8Array and filename")
		}
		bytes, err := bytesArgument(args[0], "trace bytes")
		if err != nil {
			return nil, err
		}
		name, err := stringArgument(args[1], "filename")
		if err != nil {
			return nil, err
		}
		collection, err := browsercore.ParseCollection(bytes, name)
		if err != nil {
			return nil, err
		}
		return cacheCollection(collection)
	})
	expose("rlvizDecodeAdapter", func(args []js.Value) (any, error) {
		if len(args) != 3 {
			return nil, errors.New("rlvizDecodeAdapter expects canonical bytes, filename, and source size")
		}
		bytes, err := bytesArgument(args[0], "canonical bytes")
		if err != nil {
			return nil, err
		}
		name, err := stringArgument(args[1], "filename")
		if err != nil {
			return nil, err
		}
		if args[2].Type() != js.TypeNumber || args[2].Int() < 0 {
			return nil, errors.New("source size must be a non-negative number")
		}
		collection, err := browsercore.DecodeAdapterCollection(bytes, name, args[2].Int())
		if err != nil {
			return nil, err
		}
		return cacheCollection(collection)
	})
	expose("rlvizAnalyze", func(args []js.Value) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("rlvizAnalyze expects collection digest and trajectory ID")
		}
		collection, err := cachedCollection(args[0])
		if err != nil {
			return nil, err
		}
		trajectoryID, err := stringArgument(args[1], "trajectory ID")
		if err != nil {
			return nil, err
		}
		return browsercore.Analysis(collection, trajectoryID)
	})
	expose("rlvizCompare", func(args []js.Value) (any, error) {
		if len(args) != 3 {
			return nil, errors.New("rlvizCompare expects collection digest and two trajectory IDs")
		}
		collection, err := cachedCollection(args[0])
		if err != nil {
			return nil, err
		}
		left, err := stringArgument(args[1], "left trajectory ID")
		if err != nil {
			return nil, err
		}
		right, err := stringArgument(args[2], "right trajectory ID")
		if err != nil {
			return nil, err
		}
		return browsercore.Compare(collection, left, right)
	})
	js.Global().Set("rlvizWasmLimits", map[string]any{"maxRecommendedBytes": browsercore.MaxRecommendedBytes})
	resolver := js.Global().Get("rlvizResolveWasmReady")
	if resolver.Type() == js.TypeFunction {
		resolver.Invoke()
	}
	select {}
}

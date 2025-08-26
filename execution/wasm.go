package execution

import "fmt"

// Stub runtime agar project tetap build tanpa dependensi CGO/wasmer.
type WASMRuntime struct{}

func (rt *WASMRuntime) LoadWASM(path string) error {
	// no-op stub
	return nil
}

func (rt *WASMRuntime) Call(funcName string, _ ...interface{}) (interface{}, error) {
	return nil, fmt.Errorf("WASM VM is disabled (stub). Build with -tags wasmvm to enable real VM")
}

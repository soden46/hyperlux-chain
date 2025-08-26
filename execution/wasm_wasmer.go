//go:build wasmvm

package execution

import (
	"fmt"
	"os"

	"github.com/wasmerio/wasmer-go/wasmer"
)

type WASMRuntime struct {
	engine   *wasmer.Engine
	store    *wasmer.Store
	module   *wasmer.Module
	instance *wasmer.Instance
}

func (rt *WASMRuntime) LoadWASM(path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read wasm file: %w", err)
	}
	rt.engine = wasmer.NewEngine()
	rt.store = wasmer.NewStore(rt.engine)

	module, err := wasmer.NewModule(rt.store, bytes)
	if err != nil {
		return fmt.Errorf("failed to compile wasm: %w", err)
	}
	rt.module = module

	importObject := wasmer.NewImportObject()
	inst, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		return fmt.Errorf("failed to instantiate wasm: %w", err)
	}
	rt.instance = inst
	return nil
}

func (rt *WASMRuntime) Call(funcName string, args ...interface{}) (interface{}, error) {
	if rt.instance == nil {
		return nil, fmt.Errorf("wasm instance not initialized")
	}
	fn, err := rt.instance.Exports.GetFunction(funcName)
	if err != nil {
		return nil, fmt.Errorf("function %s not found", funcName)
	}
	res, err := fn(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to call wasm function: %w", err)
	}
	return res, nil
}

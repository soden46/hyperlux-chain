package execution

import (
	"fmt"
	"os"

	"github.com/wasmerio/wasmer-go/wasmer"
)

// WASMRuntime struct untuk jalankan modul
type WASMRuntime struct {
	engine  *wasmer.Engine
	store   *wasmer.Store
	module  *wasmer.Module
	instance *wasmer.Instance
}

// LoadWASM load file wasm ke runtime
func (rt *WASMRuntime) LoadWASM(path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read wasm file: %v", err)
	}

	rt.engine = wasmer.NewEngine()
	rt.store = wasmer.NewStore(rt.engine)

	// compile
	module, err := wasmer.NewModule(rt.store, bytes)
	if err != nil {
		return fmt.Errorf("failed to compile wasm: %v", err)
	}
	rt.module = module

	// import object (kosong dulu)
	importObject := wasmer.NewImportObject()
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		return fmt.Errorf("failed to instantiate wasm: %v", err)
	}
	rt.instance = instance
	return nil
}

// Call fungsi dari wasm
func (rt *WASMRuntime) Call(funcName string, args ...interface{}) (interface{}, error) {
	function, err := rt.instance.Exports.GetFunction(funcName)
	if err != nil {
		return nil, fmt.Errorf("function %s not found", funcName)
	}

	result, err := function(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to call wasm function: %v", err)
	}
	return result, nil
}

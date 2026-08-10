package hook_func

import (
	"context"
	"reflect"
	"testing"
)

func TestTerminalFunctionsRunInReverseRegistrationOrder(t *testing.T) {
	terminalMu.Lock()
	terminalFuncList = nil
	terminalBegin = false
	terminalMu.Unlock()

	var order []string
	RegisterTerminalFunc("dependency", func(context.Context) error {
		order = append(order, "dependency")
		return nil
	})
	RegisterTerminalFunc("dependent", func(context.Context) error {
		order = append(order, "dependent")
		return nil
	})

	if errs := ExecTerminalFunc(context.Background()); len(errs) != 0 {
		t.Fatalf("ExecTerminalFunc() errors = %v", errs)
	}
	if want := []string{"dependent", "dependency"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("cleanup order = %v, want %v", order, want)
	}
}

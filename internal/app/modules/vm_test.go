package modules

import (
	"reflect"
	"testing"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
)

func TestVMModule_RegisterWorkers_RegistersVMLifecycleWorkers(t *testing.T) {
	t.Parallel()

	workers := river.NewWorkers()
	module := &VMModule{
		infra: &Infrastructure{EntClient: &ent.Client{}},
	}

	module.RegisterWorkers(workers)

	workersValue := reflect.ValueOf(workers).Elem().FieldByName("workersMap")
	if !workersValue.IsValid() {
		t.Fatal("workersMap field not found")
	}
	for _, kind := range []string{
		"vm_create",
		"vm_delete",
		"vm_modify",
		"vm_power",
		"vm_adoption_discovery_scan",
		"vm_status_sync",
		"vm_tombstone_cleanup",
	} {
		if !workersValue.MapIndex(reflect.ValueOf(kind)).IsValid() {
			t.Fatalf("worker kind %q not registered", kind)
		}
	}
}

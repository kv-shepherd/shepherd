package domain

import (
	"bytes"
	"testing"
)

func TestVMMutationSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	original := &VMMutation{
		Mode:      VMMutationModePatch,
		PatchType: VMMutationPatchTypeMerge,
		Payload:   []byte(`{"spec":{"runStrategy":"Always"}}`),
	}

	restored, err := VMMutationFromSnapshot(original.Snapshot())
	if err != nil {
		t.Fatalf("VMMutationFromSnapshot() error = %v", err)
	}
	if restored.Mode != original.Mode {
		t.Fatalf("Mode = %q, want %q", restored.Mode, original.Mode)
	}
	if restored.PatchType != original.PatchType {
		t.Fatalf("PatchType = %q, want %q", restored.PatchType, original.PatchType)
	}
	if !bytes.Equal(restored.Payload, original.Payload) {
		t.Fatalf("Payload = %q, want %q", restored.Payload, original.Payload)
	}
}

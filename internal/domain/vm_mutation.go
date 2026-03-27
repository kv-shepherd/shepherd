package domain

import "fmt"

const (
	VMMutationModePatch = "patch"

	VMMutationPatchTypeMerge = "merge"
	VMMutationPatchTypeJSON  = "json"
)

// VMMutation captures the exact mutation shape that approval dry-run and
// execution should both submit for an existing VM workflow.
type VMMutation struct {
	Mode      string `json:"mode"`
	PatchType string `json:"patch_type,omitempty"`
	Payload   []byte `json:"-"`
}

func (m *VMMutation) Snapshot() map[string]interface{} {
	if m == nil {
		return nil
	}
	return map[string]interface{}{
		"mode":       m.Mode,
		"patch_type": m.PatchType,
		"payload":    string(m.Payload),
	}
}

func VMMutationFromSnapshot(snapshot map[string]interface{}) (*VMMutation, error) {
	if len(snapshot) == 0 {
		return nil, nil
	}
	mode, _ := snapshot["mode"].(string)
	patchType, _ := snapshot["patch_type"].(string)
	payload, _ := snapshot["payload"].(string)
	if mode == "" {
		return nil, fmt.Errorf("vm mutation snapshot is missing mode")
	}
	if payload == "" {
		return nil, fmt.Errorf("vm mutation snapshot is missing payload")
	}
	return &VMMutation{
		Mode:      mode,
		PatchType: patchType,
		Payload:   []byte(payload),
	}, nil
}

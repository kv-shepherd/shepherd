package usecase

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestSameCreateResource(t *testing.T) {
	basePayload := domain.VMCreationPayload{
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "team-a",
	}

	testCases := []struct {
		name   string
		input  CreateVMInput
		expect bool
	}{
		{
			name: "same resource",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: true,
		},
		{
			name: "different service",
			input: CreateVMInput{
				ServiceID:      "svc-2",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different template",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-2",
				InstanceSizeID: "size-1",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different instance size",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-2",
				Namespace:      "team-a",
			},
			expect: false,
		},
		{
			name: "different namespace",
			input: CreateVMInput{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      "team-b",
			},
			expect: false,
		},
		{
			name: "whitespace normalized",
			input: CreateVMInput{
				ServiceID:      " svc-1 ",
				TemplateID:     "tpl-1",
				InstanceSizeID: "size-1",
				Namespace:      " team-a ",
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := sameCreateResource(basePayload, tc.input)
			if got != tc.expect {
				t.Fatalf("sameCreateResource mismatch: got %v want %v", got, tc.expect)
			}
		})
	}
}

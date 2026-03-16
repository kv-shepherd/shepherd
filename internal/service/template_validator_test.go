package service

import (
	"strings"
	"testing"
)

func TestValidateTemplateSpec_NilAndEmpty(t *testing.T) {
	if err := ValidateTemplateSpec(nil); err != nil {
		t.Fatalf("nil spec should pass validation: %v", err)
	}
	if err := ValidateTemplateSpec(map[string]interface{}{}); err != nil {
		t.Fatalf("empty spec should pass validation: %v", err)
	}
}

func TestValidateTemplateSpec_AllowedPaths(t *testing.T) {
	tests := []struct {
		name string
		spec map[string]interface{}
	}{
		{
			name: "simple image",
			spec: map[string]interface{}{
				"image": "quay.io/containerdisks/ubuntu:22.04",
			},
		},
		{
			name: "image source with pvc",
			spec: map[string]interface{}{
				"image_source": map[string]interface{}{
					"pvc_name": "ubuntu-22.04-base",
				},
			},
		},
		{
			name: "cloud-init config",
			spec: map[string]interface{}{
				"cloud_init": "#cloud-config\npackages:\n  - nginx",
			},
		},
		{
			name: "source with image",
			spec: map[string]interface{}{
				"source": map[string]interface{}{
					"image": "registry.example.com/vms/centos:9",
				},
			},
		},
		{
			name: "combined image and cloud_init",
			spec: map[string]interface{}{
				"image":      "quay.io/containerdisks/fedora:39",
				"cloud_init": "#cloud-config\nusers:\n  - name: admin",
			},
		},
		{
			name: "volumes for image sources",
			spec: map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "rootfs",
						"containerDisk": map[string]interface{}{
							"image": "quay.io/containerdisks/ubuntu:22.04",
						},
					},
				},
			},
		},
		{
			name: "pvc_name shorthand",
			spec: map[string]interface{}{
				"pvc_name": "my-base-image",
			},
		},
		{
			name: "os_family and os_version like fields",
			spec: map[string]interface{}{
				"image":     "quay.io/containerdisks/ubuntu:22.04",
				"os_family": "linux",
				"os_info":   "Ubuntu 22.04 LTS",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTemplateSpec(tt.spec); err != nil {
				t.Errorf("expected allowed spec to pass, got error: %v", err)
			}
		})
	}
}

func TestValidateTemplateSpec_ProhibitedPaths(t *testing.T) {
	tests := []struct {
		name         string
		spec         map[string]interface{}
		expectedPath string // substring expected in error message
	}{
		{
			name: "direct cpu field",
			spec: map[string]interface{}{
				"cpu": 4,
			},
			expectedPath: "cpu",
		},
		{
			name: "cpu_cores field",
			spec: map[string]interface{}{
				"cpu_cores": 8,
			},
			expectedPath: "cpu_cores",
		},
		{
			name: "memory field",
			spec: map[string]interface{}{
				"memory": "8Gi",
			},
			expectedPath: "memory",
		},
		{
			name: "memory_gi field",
			spec: map[string]interface{}{
				"memory_gi": 4,
			},
			expectedPath: "memory_gi",
		},
		{
			name: "memory_request_gi field",
			spec: map[string]interface{}{
				"memory_request_gi": 2,
			},
			expectedPath: "memory_request_gi",
		},
		{
			name: "resources.requests",
			spec: map[string]interface{}{
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"memory": "4Gi",
					},
				},
			},
			expectedPath: "resources",
		},
		{
			name: "domain.cpu nested",
			spec: map[string]interface{}{
				"domain": map[string]interface{}{
					"cpu": map[string]interface{}{
						"cores": 4,
					},
				},
			},
			expectedPath: "domain.cpu",
		},
		{
			name: "domain.resources nested",
			spec: map[string]interface{}{
				"domain": map[string]interface{}{
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "2",
							"memory": "2Gi",
						},
					},
				},
			},
			expectedPath: "domain.resources",
		},
		{
			name: "domain.devices.gpus nested",
			spec: map[string]interface{}{
				"domain": map[string]interface{}{
					"devices": map[string]interface{}{
						"gpus": []interface{}{
							map[string]interface{}{
								"name":       "gpu1",
								"deviceName": "nvidia.com/GP102GL_Tesla_P40",
							},
						},
					},
				},
			},
			expectedPath: "domain.devices.gpus",
		},
		{
			name: "requires_gpu flag",
			spec: map[string]interface{}{
				"requires_gpu": true,
			},
			expectedPath: "requires_gpu",
		},
		{
			name: "requires_sriov flag",
			spec: map[string]interface{}{
				"requires_sriov": true,
			},
			expectedPath: "requires_sriov",
		},
		{
			name: "dedicated_cpu flag",
			spec: map[string]interface{}{
				"dedicated_cpu": true,
			},
			expectedPath: "dedicated_cpu",
		},
		{
			name: "hugepages_size field",
			spec: map[string]interface{}{
				"hugepages_size": "2Mi",
			},
			expectedPath: "hugepages_size",
		},
		{
			name: "disk_gb field",
			spec: map[string]interface{}{
				"disk_gb": 100,
			},
			expectedPath: "disk_gb",
		},
		{
			name: "mixed allowed and prohibited - prohibited detected",
			spec: map[string]interface{}{
				"image":  "quay.io/containerdisks/ubuntu:22.04",
				"cpu":    4,
				"memory": "8Gi",
			},
			expectedPath: "", // either cpu or memory will be caught
		},
		{
			name: "deep nesting prohibited - full kubevirt path",
			spec: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{
									"cores": 4,
								},
							},
						},
					},
				},
			},
			expectedPath: "spec.template.spec.domain.cpu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateSpec(tt.spec)
			if err == nil {
				t.Error("expected validation error for prohibited path, got nil")
				return
			}
			if !strings.Contains(err.Error(), "prohibited hardware path") {
				t.Errorf("expected error to mention 'prohibited hardware path', got: %v", err)
			}
			if !strings.Contains(err.Error(), "InstanceSize") {
				t.Errorf("expected error to mention 'InstanceSize', got: %v", err)
			}
			if tt.expectedPath != "" && !strings.Contains(err.Error(), tt.expectedPath) {
				t.Errorf("expected error to mention path %q, got: %v", tt.expectedPath, err)
			}
		})
	}
}

func TestIsProhibitedTemplatePath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Prohibited
		{"cpu", true},
		{"CPU", true}, // case insensitive
		{"cpu_cores", true},
		{"memory", true},
		{"memory_gi", true},
		{"memory_request_gi", true},
		{"resources", true},
		{"resources.requests", true},
		{"resources.limits.memory", true},
		{"domain.cpu", true},
		{"domain.cpu.cores", true},
		{"domain.resources", true},
		{"domain.memory.hugepages", true},
		{"domain.devices.gpus", true},
		{"domain.devices.interfaces", true},
		{"requires_gpu", true},
		{"requires_sriov", true},
		{"dedicated_cpu", true},
		{"hugepages_size", true},
		{"overcommit", true},

		// Allowed
		{"image", false},
		{"image_source", false},
		{"cloud_init", false},
		{"cloudInit", false},
		{"source", false},
		{"volumes", false},
		{"pvc_name", false},
		{"os_family", false},
		{"description", false},
		{"", false},
		{"  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isProhibitedTemplatePath(tt.path)
			if result != tt.expected {
				t.Errorf("isProhibitedTemplatePath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

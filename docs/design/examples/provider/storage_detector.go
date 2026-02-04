// Package provider provides storage detection utilities.
//
// Reference: ADR-0015 §8, 04-governance.md §5.5
// Purpose: Cluster StorageClass auto-detection during health checks
// V1 Strategy: Auto-detect available StorageClasses, admin can set default

package provider

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// StorageDetector handles StorageClass detection for clusters
type StorageDetector struct {
	clusterRepo ClusterRepository
}

// ClusterRepository defines the interface for cluster data access
type ClusterRepository interface {
	UpdateStorageClasses(ctx context.Context, clusterID string, classes []string, defaultClass string) error
}

// NewStorageDetector creates a new storage detector
func NewStorageDetector(clusterRepo ClusterRepository) *StorageDetector {
	return &StorageDetector{clusterRepo: clusterRepo}
}

// DetectStorageClasses updates cluster's storage class list
// Called during periodic health checks (60s interval)
func (d *StorageDetector) DetectStorageClasses(ctx context.Context,
	clusterID string, restConfig *rest.Config) error {

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create k8s client: %w", err)
	}

	scList, err := clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list storage classes: %w", err)
	}

	var names []string
	var defaultSC string
	for _, sc := range scList.Items {
		names = append(names, sc.Name)
		// Check for default annotation
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultSC = sc.Name
		}
	}

	// Update cluster record in database
	return d.clusterRepo.UpdateStorageClasses(ctx, clusterID, names, defaultSC)
}

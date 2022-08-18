/*
Copyright 2022 openBCE.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// KinkControlPlaneSpec defines the desired state of KinkControlPlane
type KinkControlPlaneSpec struct {
	// Kubeconf is the credential used to access tenant kubernetes master.
	Kubeconf string `json:"kubeconf,omitempty"`

	// CredentialsName is the credential for worker to join tenant kubernetes master.
	CredentialsName string `json:"credentialsName,omitempty"`

	// ClusterName is the name of cluster.
	ClusterName string `json:"clusterName,omitempty"`

	// Version is the version of kubernetes for the cluster.
	Version *string `json:"version,omitempty"`
}

// KinkControlPlaneStatus defines the observed state of KinkControlPlane
type KinkControlPlaneStatus struct {
	// Version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	Version *string `json:"version,omitempty"`

	// Total number of fully running and ready control plane machines.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// Initialized denotes whether or not the control plane has the
	// uploaded kubeconf configmap.
	// +optional
	Initialized bool `json:"initialized,omitempty"`

	// Ready denotes that the KinkControlPlane API Server is ready to
	// receive requests.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// FailureReason indicates that there is a terminal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// ErrorMessage indicates that there is a terminal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the KinkControlPlane.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// KinkControlPlane is the Schema for the kinkcontrolplanes API
type KinkControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KinkControlPlaneSpec   `json:"spec,omitempty"`
	Status KinkControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KinkControlPlaneList contains a list of KinkControlPlane
type KinkControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KinkControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KinkControlPlane{}, &KinkControlPlaneList{})
}

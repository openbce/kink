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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type ControlPlaneRole string

const (
	ControlPlaneRoleLabelName = "kink.openbce.io/role"

	ApiServer ControlPlaneRole = "apiserver"
	ETCD      ControlPlaneRole = "etcd"
	Unkonwn   ControlPlaneRole = "unknown"
)

// KinkMachineSpec defines the desired state of KinkMachine
type KinkMachineSpec struct {
	// Version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	Version *string `json:"version,omitempty"`
}

// KinkMachineStatus defines the observed state of KinkMachine
type KinkMachineStatus struct {
	// Pods includes the control plane pods that managed by the KinkControlPlane.
	// +optional
	Pods []v1.ObjectReference `json:"pods,omitempty"`

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

// KinkMachine is the Schema for the kinkmachines API
type KinkMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KinkMachineSpec   `json:"spec,omitempty"`
	Status KinkMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion

// KinkMachineList contains a list of KinkMachine
type KinkMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KinkMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KinkMachine{}, &KinkMachineList{})
}

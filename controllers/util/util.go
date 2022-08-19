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

package util

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"openbce.io/kink/apis/infrastructure/v1alpha1"
)

func GetControlPlaneRole(pod *metav1.ObjectMeta) (v1alpha1.ControlPlaneRole, error) {
	if pod == nil || pod.Labels == nil {
		return v1alpha1.Unkonwn, fmt.Errorf("pod is nil")
	}

	podType, found := pod.Labels[v1alpha1.ControlPlaneRoleLabelName]
	if !found {
		return v1alpha1.Unkonwn, fmt.Errorf("pod is nil")
	}

	return v1alpha1.ControlPlaneRole(podType), nil
}

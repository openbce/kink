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

package templates

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/util/secret"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	infrav1alpha1 "openbce.io/kink/apis/infrastructure/v1alpha1"
)

const (
	EtcdDefaultPort = 2379
)

func EtcdServiceTemplate(cluster *clusterv1.Cluster, machine *infrav1alpha1.KinkMachine) *v1.Service {
	owner := metav1.OwnerReference{
		APIVersion:         infrav1alpha1.GroupVersion.String(),
		Kind:               "KinkMachine",
		Name:               machine.Name,
		UID:                machine.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}

	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-etcd-svc",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName:              cluster.Name,
				infrav1alpha1.ControlPlaneRoleLabelName: string(infrav1alpha1.ETCD),
			},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port: EtcdDefaultPort,
					TargetPort: intstr.IntOrString{
						IntVal: EtcdDefaultPort,
						Type:   intstr.Int,
					},
				},
			},
			Selector: nil,
			Type:     v1.ServiceTypeClusterIP,
		},
	}
}

func EtcdPodTemplate(cluster *clusterv1.Cluster, machine *infrav1alpha1.KinkMachine) *v1.Pod {
	owner := metav1.OwnerReference{
		APIVersion:         infrav1alpha1.GroupVersion.String(),
		Kind:               "KinkMachine",
		Name:               machine.Name,
		UID:                machine.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}

	caName := fmt.Sprintf("%s-%s", cluster.Name, secret.ClusterCA)

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(cluster.Name + "-etcd-"),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName:              cluster.Name,
				infrav1alpha1.ControlPlaneRoleLabelName: string(infrav1alpha1.ETCD),
			},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
			HostNetwork:   true,
			Containers: []v1.Container{
				{
					Image: "openbce/etcd:3.5.3-0",
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      caName,
							MountPath: secret.DefaultCertificatesDir,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: caName,
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: caName,
						},
					},
				},
			},
		},
	}
}

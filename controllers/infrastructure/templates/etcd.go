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
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	infrav1beta1 "openbce.io/kink/apis/infrastructure/v1beta1"
)

const (
	EtcdDefaultPort = 2379
)

func EtcdServiceTemplate(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) *v1.Service {
	owner := metav1.OwnerReference{
		APIVersion:         infrav1beta1.GroupVersion.String(),
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
				clusterv1.ClusterLabelName:             cluster.Name,
				infrav1beta1.ControlPlaneRoleLabelName: string(infrav1beta1.ETCD),
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
			Selector: map[string]string{
				clusterv1.ClusterLabelName:             cluster.Name,
				infrav1beta1.ControlPlaneRoleLabelName: string(infrav1beta1.ETCD),
			},
			Type: v1.ServiceTypeClusterIP,
		},
	}
}

func EtcdPodTemplate(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) *v1.Pod {
	owner := metav1.OwnerReference{
		APIVersion:         infrav1beta1.GroupVersion.String(),
		Kind:               "KinkMachine",
		Name:               machine.Name,
		UID:                machine.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}

	volumes, mounts := getSecretVolumes(cluster)

	podName := names.SimpleNameGenerator.GenerateName(cluster.Name + "-etcd-")

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName:             cluster.Name,
				infrav1beta1.ControlPlaneRoleLabelName: string(infrav1beta1.ETCD),
			},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
			HostNetwork:   true,
			Containers: []v1.Container{
				{
					Name:    "etcd",
					Image:   "openbce/etcd:3.5.3-0",
					Env:     []v1.EnvVar{hostIPEnvVar},
					Command: []string{"/bin/sh", "-c"},
					Args: []string{strings.Join([]string{
						"etcd",
						fmt.Sprintf("--advertise-client-urls=http://${host_ip}:%d", EtcdDefaultPort),
						fmt.Sprintf("--listen-client-urls=http://${host_ip}:%d,http://127.0.0.1:%d", EtcdDefaultPort, EtcdDefaultPort)},
						" "),
					},
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
}

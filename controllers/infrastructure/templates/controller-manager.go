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
	"k8s.io/apiserver/pkg/storage/names"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	infrav1beta1 "openbce.io/kink/apis/infrastructure/v1beta1"
)

func ControllerManagerPodTemplate(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) *v1.Pod {
	owner := metav1.NewControllerRef(machine,
		infrav1beta1.GroupVersion.WithKind("KinkMachine"))

	volumes, mounts := getSecretVolumes(cluster)

	serviceDIDR := "192.168.0.0/24"
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) != 0 {
		serviceDIDR = cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
	}

	podCIDR := "192.168.100.0/24"
	if len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) != 0 {
		podCIDR = cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]
	}

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(cluster.Name + "-controller-manager-"),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName:             cluster.Name,
				infrav1beta1.ControlPlaneRoleLabelName: string(infrav1beta1.ControllerManager),
			},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
			HostNetwork:   true,
			DNSPolicy:     v1.DNSClusterFirstWithHostNet,
			Containers: []v1.Container{
				{
					Name:    "controller-manager",
					Image:   "openbce/kube-controller-manager:v1.24.1",
					Env:     []v1.EnvVar{hostIPEnvVar},
					Command: []string{"/bin/sh", "-c"},
					Args: []string{strings.Join([]string{
						"kube-controller-manager",
						"--allocate-node-cidrs=true",
						"--authentication-kubeconfig=/etc/kubernetes/controller-manager.conf",
						"--authorization-kubeconfig=/etc/kubernetes/controller-manager.conf",
						"--bind-address=${host_ip}",
						"--client-ca-file=/etc/kubernetes/pki/ca.crt",
						fmt.Sprintf("--cluster-cidr=%s", podCIDR),
						"--cluster-name=kubernetes",
						"--cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt",
						"--cluster-signing-key-file=/etc/kubernetes/pki/ca.key",
						"--controllers=*,bootstrapsigner,tokencleaner",
						"--kubeconfig=/etc/kubernetes/controller-manager.conf",
						"--leader-elect=true",
						"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
						"--root-ca-file=/etc/kubernetes/pki/ca.crt",
						"--service-account-private-key-file=/etc/kubernetes/pki/sa.key",
						fmt.Sprintf("--service-cluster-ip-range=%s", serviceDIDR),
						"--use-service-account-credentials=true",
					},
						" "),
					},
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
}

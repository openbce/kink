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

func SchedulerPodTemplate(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) *v1.Pod {
	owner := metav1.NewControllerRef(machine,
		infrav1beta1.GroupVersion.WithKind("KinkMachine"))

	volumes, mounts := getSecretVolumes(cluster)

	serviceDIDR := "192.168.0.0/24"
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) != 0 {
		serviceDIDR = cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
	}

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(cluster.Name + "-apiserver-"),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName:             cluster.Name,
				infrav1beta1.ControlPlaneRoleLabelName: string(infrav1beta1.ApiServer),
			},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
			HostNetwork:   true,
			DNSPolicy:     v1.DNSClusterFirstWithHostNet,
			Containers: []v1.Container{
				{
					Name:    "apiserver",
					Image:   "openbce/kube-scheduler:v1.24.1",
					Env:     []v1.EnvVar{hostIPEnvVar},
					Command: []string{"/bin/sh", "-c"},
					Args: []string{strings.Join([]string{
						"kube-apiserver",
						"--advertise-address=${host_ip}",
						"--secure-port=6443",
						fmt.Sprintf("--etcd-servers=http://%s-etcd-svc.%s:%d",
							cluster.Name, cluster.Namespace, EtcdDefaultPort),
						fmt.Sprintf("--service-cluster-ip-range=%s", serviceDIDR),

						"--allow-privileged=true",
						"--authorization-mode=Node,RBAC",
						"--enable-admission-plugins=NodeRestriction",
						"--enable-bootstrap-token-auth=true",
						"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
						"--requestheader-allowed-names=front-proxy-client",
						"--requestheader-extra-headers-prefix=X-Remote-Extra-",
						"--requestheader-group-headers=X-Remote-Group",
						"--requestheader-username-headers=X-Remote-User",

						"--client-ca-file=/etc/kubernetes/pki/ca/tls.crt",
						"--requestheader-client-ca-file=/etc/kubernetes/pki/ca/tls.crt",
						"--proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-ca/tls.crt",
						"--proxy-client-key-file=/etc/kubernetes/pki/front-proxy-ca/tls.key",
						"--kubelet-client-certificate=/etc/kubernetes/pki/kubelet-client/tls.crt",
						"--kubelet-client-key=/etc/kubernetes/pki/kubelet-client/tls.key",
						"--service-account-issuer=https://kubernetes.default.svc.cluster.local",
						"--service-account-key-file=/etc/kubernetes/pki/sa/tls.crt",
						"--service-account-signing-key-file=/etc/kubernetes/pki/sa/tls.key",
						"--tls-cert-file=/etc/kubernetes/pki/apiserver/tls.crt",
						"--tls-private-key-file=/etc/kubernetes/pki/apiserver/tls.key"},
						" "),
					},
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
}

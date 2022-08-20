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

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api/util/secret"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var hostIPEnvVar = v1.EnvVar{
	Name: "host_ip",
	ValueFrom: &v1.EnvVarSource{
		FieldRef: &v1.ObjectFieldSelector{
			FieldPath: "status.podIP",
		},
	},
}

func getSecretVolumes(cluster *clusterv1.Cluster) ([]v1.Volume, []v1.VolumeMount) {
	var volumes []v1.Volume
	var mounts []v1.VolumeMount

	certs := []secret.Purpose{
		secret.ClusterCA,
		secret.FrontProxyCA,
		secret.ServiceAccount,
	}

	// Add certs
	for _, cert := range certs {
		certName := fmt.Sprintf("%s-%s", cluster.Name, cert)

		volume := v1.Volume{
			Name: certName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: certName,
				},
			},
		}
		volumes = append(volumes, volume)

		mount := v1.VolumeMount{
			Name:      certName,
			MountPath: secret.DefaultCertificatesDir + "/" + string(cert),
		}
		mounts = append(mounts, mount)
	}

	// Add root ca config map
	caPath, caName := "root", "kube-root-ca.crt"
	volume := v1.Volume{
		Name: caPath,
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: caName,
				},
			},
		},
	}
	volumes = append(volumes, volume)

	mount := v1.VolumeMount{
		Name:      caPath,
		MountPath: secret.DefaultCertificatesDir + "/" + caPath,
	}
	mounts = append(mounts, mount)

	return volumes, mounts
}

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

package secrets

import (
	"context"
	"crypto/rsa"
	"crypto/x509"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"openbce.io/kink/apis/controlplane/v1beta1"
)

// NewCertificatesManager will create a cert manager to generate all related CAs for the master.
// The struct of CAs are described as following:
//   root-ca: the root CA of all crt/keys
//     apiserver-ca: the crt/key for client to connect, the client-ca-file will reuse root ca
//     front-proxy-ca:
//     kubelet-ca:
//     sa-ca:
func NewCertificatesManager(ctx context.Context, r client.Client, cluster *clusterv1.Cluster, kcp *v1beta1.KinkControlPlane) *CertificatesManager {
	return &CertificatesManager{
		ctx:     ctx,
		r:       r,
		kcp:     kcp,
		cluster: cluster,
	}
}

type CertificatesManager struct {
	ctx     context.Context
	r       client.Client
	kcp     *v1beta1.KinkControlPlane
	cluster *clusterv1.Cluster
}

// LookupOrGenerateCAs will generate both Server and Client CAs for the cluster.
func (c *CertificatesManager) LookupOrGenerateCAs() error {
	certTree, err := GetCerts().AsMap().CertTree()
	if err != nil {
		return err
	}
	if err = certTree.CreateTree(c.ctx, c.r, c.kcp, c.cluster); err != nil {
		return err
	}

	// generate sa-ca private/public key
	// The key does NOT exist, let's generate it now
	key, err := pkiutil.NewPrivateKey(x509.RSA)
	if err != nil {
		return err
	}

	pub := key.Public()
	if err = createSASecret(c.ctx, c.r, c.kcp, c.cluster,
		"sa", pub.(*rsa.PublicKey), key.(*rsa.PrivateKey)); err != nil {
		return err
	}

	return nil
}

func (c *CertificatesManager) LookupOrGenerateKubeconfig(kcp *v1beta1.KinkControlPlane, cluster *clusterv1.Cluster) error {
	clusterName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	endpoint := cluster.Spec.ControlPlaneEndpoint

	ownerRef := metav1.NewControllerRef(kcp,
		v1beta1.GroupVersion.WithKind("KinkControlPlane"))

	err := kubeconfig.CreateSecretWithOwner(c.ctx, c.r, clusterName, endpoint.String(), *ownerRef)
	if err != nil {
		return err
	}

	return nil
}

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

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrlv1beta1 "openbce.io/kink/apis/controlplane/v1beta1"
)

// NewCertificatesManager will create a cert manager to generate all related CAs for the master.
// The struct of CAs are described as following:
//   root-ca: the root CA of all crt/keys
//     apiserver-ca: the crt/key for client to connect, the client-ca-file will reuse root ca
//     front-proxy-ca:
//     kubelet-ca:
//     sa-ca:
func NewCertificatesManager(ctx context.Context, r client.Client, cluster *clusterv1.Cluster, kcp *ctrlv1beta1.KinkControlPlane) *CertificatesManager {
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
	kcp     *ctrlv1beta1.KinkControlPlane
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

func (c *CertificatesManager) LookupOrGenerateKubeconfig() error {
	clusterName := types.NamespacedName{
		Namespace: c.cluster.Namespace,
		Name:      c.cluster.Name,
	}

	kcName := types.NamespacedName{
		Namespace: c.cluster.Namespace,
		Name:      secret.Name(c.cluster.Name, secret.Kubeconfig),
	}

	kc := &v1.Secret{}
	if err := c.r.Get(c.ctx, kcName, kc); err != nil {
		if apierrors.IsNotFound(err) {
			ownerRef := metav1.NewControllerRef(c.kcp,
				ctrlv1beta1.GroupVersion.WithKind("KinkControlPlane"))

			endpoint := c.cluster.Spec.ControlPlaneEndpoint
			if err := kubeconfig.CreateSecretWithOwner(c.ctx, c.r, clusterName, endpoint.String(), *ownerRef); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

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
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"

	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"

	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"
	netutils "k8s.io/utils/net"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "openbce.io/kink/apis/controlplane/v1beta1"
)

var certNameFmt = "%s-%s"

type configMutatorsFunc func(cfg *pkiutil.CertConfig, cluster *clusterv1.Cluster)

type KinkCert struct {
	root *KinkCert

	Name     string
	LongName string
	BaseName string
	CAName   string

	config         pkiutil.CertConfig
	configMutators []configMutatorsFunc
}

func (c *KinkCert) GetConfig(cluster *clusterv1.Cluster) (*pkiutil.CertConfig, error) {
	for _, f := range c.configMutators {
		f(&c.config, cluster)
	}

	c.config.PublicKeyAlgorithm = x509.RSA
	return &c.config, nil
}

func createCASecret(ctx context.Context, r client.Client,
	kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster,
	k *KinkCert, crt *x509.Certificate, key crypto.Signer) error {

	caName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      fmt.Sprintf(certNameFmt, cluster.Name, k.Name),
	}

	sec := &v1.Secret{}
	if err := r.Get(ctx, caName, sec); err != nil {
		if apierrors.IsNotFound(err) {
			sec = buildCertSecret(kcp, cluster, k, crt, key)

			if err = r.Create(ctx, sec); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func createSASecret(ctx context.Context, r client.Client,
	kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster,
	name string, pub *rsa.PublicKey, key *rsa.PrivateKey) error {

	caName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      fmt.Sprintf(certNameFmt, cluster.Name, name),
	}

	sec := &v1.Secret{}
	if err := r.Get(ctx, caName, sec); err != nil {
		if apierrors.IsNotFound(err) {
			sec = buildSASecret(kcp, cluster, name, pub, key)

			if err = r.Create(ctx, sec); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func buildSASecret(kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster,
	name string, crt *rsa.PublicKey, key *rsa.PrivateKey) *v1.Secret {
	controllerRef := metav1.NewControllerRef(kcp,
		controlplanev1.GroupVersion.WithKind("KinkControlPlane"))

	pub, err := certs.EncodePublicKeyPEM(crt)
	if err != nil {
		return nil
	}

	sec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(certNameFmt, cluster.Name, name),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: cluster.Name,
			},

			OwnerReferences: []metav1.OwnerReference{*controllerRef},
		},
		Data: map[string][]byte{
			secret.TLSCrtDataName: pub,
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(key),
		},
		Type: clusterv1.ClusterSecretType,
	}
	return sec
}

func buildCertSecret(kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster, k *KinkCert, crt *x509.Certificate, key crypto.Signer) *v1.Secret {
	controllerRef := metav1.NewControllerRef(kcp,
		controlplanev1.GroupVersion.WithKind("KinkControlPlane"))

	sec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(certNameFmt, cluster.Name, k.Name),
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*controllerRef},
		},
		Data: map[string][]byte{
			secret.TLSCrtDataName: certs.EncodeCertPEM(crt),
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(key.(*rsa.PrivateKey)),
		},
		Type: clusterv1.ClusterSecretType,
	}
	return sec
}

// CreateFromCA makes and writes a certificate using the given CA cert and key.
func (k *KinkCert) CreateFromCA(ctx context.Context, r client.Client, kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster, caCert *x509.Certificate, caKey crypto.Signer) error {
	cfg, err := k.GetConfig(cluster)
	if err != nil {
		return errors.Wrapf(err, "couldn't create %q certificate", k.Name)
	}
	cert, key, err := pkiutil.NewCertAndKey(caCert, caKey, cfg)
	if err != nil {
		return err
	}

	if err := createCASecret(ctx, r, kcp, cluster, k, cert, key); err != nil {
		return errors.Wrapf(err, "failed to write or validate certificate %s/%q", cluster.Name, k.Name)
	}

	return nil
}

// CertificateTree is represents a one-level-deep tree, mapping a CA to the certs that depend on it.
type CertificateTree map[*KinkCert]Certificates

// CreateTree creates the CAs, certs signed by the CAs, and writes them all to disk.
func (t CertificateTree) CreateTree(ctx context.Context, r client.Client, kcp *controlplanev1.KinkControlPlane, cluster *clusterv1.Cluster) error {
	for ca, leaves := range t {
		cfg, err := ca.GetConfig(cluster)
		if err != nil {
			return err
		}

		// CACert doesn't already exist, create a new cert and key.
		caCert, caKey, err := pkiutil.NewCertificateAuthority(cfg)
		if err != nil {
			return err
		}

		if err := createCASecret(ctx, r, kcp, cluster, ca, caCert, caKey); err != nil {
			return errors.Wrapf(err, "failed to write or validate certificate %s/%q", cluster.Name, ca.Name)
		}

		for _, leaf := range leaves {
			if err := leaf.CreateFromCA(ctx, r, kcp, cluster, caCert, caKey); err != nil {
				return err
			}
		}
	}
	return nil
}

// CertificateMap is a flat map of certificates, keyed by Name.
type CertificateMap map[string]*KinkCert

// CertTree returns a one-level-deep tree, mapping a CA cert to an array of certificates that should be signed by it.
func (m CertificateMap) CertTree() (CertificateTree, error) {
	caMap := make(CertificateTree)

	for _, cert := range m {
		if cert.CAName == "" {
			if _, ok := caMap[cert]; !ok {
				caMap[cert] = []*KinkCert{}
			}
		} else {
			ca, ok := m[cert.CAName]
			if !ok {
				return nil, errors.Errorf("certificate %q references unknown CA %q", cert.Name, cert.CAName)
			}
			caMap[ca] = append(caMap[ca], cert)
		}
	}

	return caMap, nil
}

// AsMap returns the list of certificates as a map, keyed by name.
func (c Certificates) AsMap() CertificateMap {
	certMap := make(map[string]*KinkCert)
	for _, cert := range c {
		certMap[cert.Name] = cert
	}

	return certMap
}

type Certificates []*KinkCert

// GetCerts returns all of the certificates kubeadm needs when etcd is hosted externally.
func GetCerts() Certificates {
	return Certificates{
		KinkCertRootCA(),
		KinkCertAPIServer(),
		KinkCertKubeletClient(),
		// Front Proxy certs
		KinkCertFrontProxyCA(),
		KinkCertFrontProxyClient(),
	}
}

// KinkCertRootCA is the definition of the Kubernetes Root CA for the API Server and kubelet.
func KinkCertRootCA() *KinkCert {
	return &KinkCert{
		Name:     "ca",
		LongName: "self-signed Kubernetes CA to provision identities for other Kubernetes components",
		BaseName: kubeadmconstants.CACertAndKeyBaseName,
		config: pkiutil.CertConfig{
			Config: certutil.Config{
				CommonName: "kubernetes",
			},
		},
	}
}

// KinkCertAPIServer is the definition of the cert used to serve the Kubernetes API.
func KinkCertAPIServer() *KinkCert {
	return &KinkCert{
		Name:     "apiserver",
		LongName: "KinkCert for serving the Kubernetes API",
		BaseName: kubeadmconstants.APIServerCertAndKeyBaseName,
		CAName:   "ca",
		config: pkiutil.CertConfig{
			Config: certutil.Config{
				CommonName: kubeadmconstants.APIServerCertCommonName,
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			},
		},
		configMutators: []configMutatorsFunc{
			func(cfg *pkiutil.CertConfig, cluster *clusterv1.Cluster) {
				if altNames, err := getAPIServerAltNames(cluster); err != nil {
					cfg.AltNames = *altNames
				}
			},
		},
	}
}

func getAPIServerAltNames(cluster *clusterv1.Cluster) (*certutil.AltNames, error) {
	svcSubnet := "192.168.0.0/24"
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		svcSubnet = cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
	}

	internalAPIServerVirtualIP, err := kubeadmconstants.GetAPIServerVirtualIP(svcSubnet)
	if err != nil {
		return nil, err
	}

	// create AltNames with defaults DNSNames/IPs
	altNames := &certutil.AltNames{
		DNSNames: []string{
			"kubernetes",
			"kubernetes.default",
			"kubernetes.default.svc",
			fmt.Sprintf("kubernetes.default.svc.%s", cluster.Spec.ClusterNetwork.ServiceDomain),
		},
		IPs: []net.IP{
			internalAPIServerVirtualIP,
		},
	}

	// add cluster controlPlaneEndpoint if present (dns or ip)
	if cluster.Spec.ControlPlaneEndpoint.IsValid() {
		host := cluster.Spec.ControlPlaneEndpoint.Host
		if ip := netutils.ParseIPSloppy(host); ip != nil {
			altNames.IPs = append(altNames.IPs, ip)
		} else {
			altNames.DNSNames = append(altNames.DNSNames, host)
		}
	}

	return altNames, nil
}

// KinkCertKubeletClient is the definition of the cert used by the API server to access the kubelet.
func KinkCertKubeletClient() *KinkCert {
	return &KinkCert{
		Name:     "apiserver-kubelet-client",
		LongName: "KinkCert for the API server to connect to kubelet",
		BaseName: kubeadmconstants.APIServerKubeletClientCertAndKeyBaseName,
		CAName:   "ca",
		config: pkiutil.CertConfig{
			Config: certutil.Config{
				CommonName:   kubeadmconstants.APIServerKubeletClientCertCommonName,
				Organization: []string{kubeadmconstants.SystemPrivilegedGroup},
				Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			},
		},
	}
}

// KinkCertFrontProxyCA is the definition of the CA used for the front end proxy.
func KinkCertFrontProxyCA() *KinkCert {
	return &KinkCert{
		Name:     "front-proxy-ca",
		LongName: "self-signed CA to provision identities for front proxy",
		BaseName: kubeadmconstants.FrontProxyCACertAndKeyBaseName,
		config: pkiutil.CertConfig{
			Config: certutil.Config{
				CommonName: "front-proxy-ca",
			},
		},
	}
}

// KinkCertFrontProxyClient is the definition of the cert used by the API server to access the front proxy.
func KinkCertFrontProxyClient() *KinkCert {
	return &KinkCert{
		Name:     "front-proxy-client",
		BaseName: kubeadmconstants.FrontProxyClientCertAndKeyBaseName,
		LongName: "KinkCert for the front proxy client",
		CAName:   "front-proxy-ca",
		config: pkiutil.CertConfig{
			Config: certutil.Config{
				CommonName: kubeadmconstants.FrontProxyClientCertCommonName,
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			},
		},
	}
}

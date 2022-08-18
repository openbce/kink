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

package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1beta1 "openbce.io/kink/apis/controlplane/v1beta1"
)

// KinkControlPlaneReconciler reconciles a KinkControlPlane object
type KinkControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KinkControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: get KinkControlPlane instance
	kcp := &controlplanev1beta1.KinkControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	cluster := &clusterv1.Cluster{}
	clusterKey := types.NamespacedName{
		Namespace: kcp.Namespace,
		Name:      kcp.Spec.ClusterName,
	}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		logger.Error(err, "Failed to get cluster for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 2: get endpoint & credential of tenant kubernetes cluster
	kcs := &v1.Secret{}
	secretKey := types.NamespacedName{
		Name:      kcp.Spec.Kubeconf,
		Namespace: kcp.Namespace,
	}
	if err := r.Client.Get(ctx, secretKey, kcs); err != nil {
		logger.Error(err, "Failed to get credential for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 3: build the client of tenant k8s cluster
	kubeconfData, found := kcs.Data["kubeconf"]
	if !found {
		logger.Error(fmt.Errorf("no data found in map"), "Failed to get kubeconf for KinkControlPlane in secret", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}
	kubeconfPath, err := persistKubeConf(kubeconfData)
	if err != nil {
		logger.Error(err, "Failed to persist kubeconf for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}
	defer os.Remove(*kubeconfPath)

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfPath)
	if err != nil {
		logger.Error(err, "Failed to create rest-config for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Failed to create clientset for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Error(err, "Failed to get server version from tenant kubernetes cluster")
		return ctrl.Result{Requeue: true}, nil
	}

	ver := fmt.Sprintf("%s.%s", info.Major, info.Minor)
	kcp.Status.Version = &ver

	// TODO: update according to master node.
	kcp.Status.ReadyReplicas = 1
	kcp.Status.UnavailableReplicas = 0

	// Step 4: get data from tenant k8s cluster and update status accordingly
	if kcp.Status.ReadyReplicas > 0 {
		kcp.Status.Initialized = true
		kcp.Status.Ready = true
	} else {
		reason := "no enough ready control planes"

		kcp.Status.FailureReason = &reason
		kcp.Status.FailureMessage = &reason
	}

	if err := r.Status().Update(ctx, kcp); err != nil {
		logger.Error(err, "Failed to status of KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	_, err = secret.Get(ctx, r.Client, util.ObjectKey(cluster), secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		kcs := kubeconfig.GenerateSecret(cluster, kubeconfData)
		if err := r.Client.Create(ctx, kcs); err != nil {
			logger.Error(err, "Failed to create kubeconf secret for tenant k8s cluster", "kubeconf", kcs)
			return ctrl.Result{Requeue: true}, nil
		}
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", cluster.Name, cluster.Namespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KinkControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1beta1.KinkControlPlane{}).
		Complete(r)
}

func persistKubeConf(data []byte) (*string, error) {
	// Create our Temp File:  This will create a filename like /tmp/prefix-123456
	// We can use a pattern of "pre-*.txt" to get an extension like: /tmp/pre-123456.txt
	tmpFile, err := ioutil.TempFile(os.TempDir(), "kubeconf-")
	if err != nil {
		return nil, err
	}

	defer func() {
		tmpFile.Close()
	}()

	if _, err = tmpFile.Write(data); err != nil {
		return nil, err
	}

	kc := tmpFile.Name()
	return &kc, nil
}

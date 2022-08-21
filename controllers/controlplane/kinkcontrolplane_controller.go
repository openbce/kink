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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/pointer"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"

	ctrlv1beta1 "openbce.io/kink/apis/controlplane/v1beta1"
	infrav1beta1 "openbce.io/kink/apis/infrastructure/v1beta1"
)

// KinkControlPlaneReconciler reconciles a KinkControlPlane object
type KinkControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kinkcontrolplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KinkControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: get KinkControlPlane instance
	kcp := &ctrlv1beta1.KinkControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	cluster := &clusterv1.Cluster{}
	clusterName := types.NamespacedName{
		Namespace: kcp.Namespace,
		Name:      kcp.Spec.ClusterName,
	}
	if err := r.Get(ctx, clusterName, cluster); err != nil {
		logger.Error(err, "Failed to get cluster for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	if !cluster.Status.InfrastructureReady {
		logger.Info("Waiting for cluster infrastructure ready.", "cluster", cluster)
		return ctrl.Result{}, nil
	}

	// Step 2: generate CA & kubeconf for control plane & data plane
	certs := secret.NewCertificatesForInitialControlPlane(nil)
	err := certs.LookupOrGenerate(ctx, r.Client, clusterName,
		*metav1.NewControllerRef(kcp, ctrlv1beta1.GroupVersion.WithKind("KinkControlPlane")))
	if err != nil {
		logger.Error(err, "Failed to find certifications for KinkControlPlane", "KinkControlPlane", kcp)
		return ctrl.Result{Requeue: true}, nil
	}

	_, err = secret.Get(ctx, r.Client, util.ObjectKey(cluster), secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		if err := kubeconfig.CreateSecret(ctx, r.Client, cluster); err != nil {
			return ctrl.Result{Requeue: true}, errors.Wrapf(err, "failed to create Kubeconfig for Cluster %q in namespace %q", cluster.Name, cluster.Namespace)
		}
	case err != nil:
		return ctrl.Result{Requeue: true}, errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", cluster.Name, cluster.Namespace)
	}

	// Step 3: lookup or create KinkMachine of this KinkControlPlane
	machines, err := r.lookupOrCreateMachines(ctx, cluster, kcp)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// Step 4: update KinkControlPlane's status accordingly
	if err := r.updateKinkCtlPlaneStatus(ctx, kcp, machines); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

func (r *KinkControlPlaneReconciler) lookupOrCreateMachines(ctx context.Context, cluster *clusterv1.Cluster, kcp *ctrlv1beta1.KinkControlPlane) (*infrav1beta1.KinkMachineList, error) {
	logger := log.FromContext(ctx)

	kms := &infrav1beta1.KinkMachineList{}
	if err := r.Client.List(ctx, kms,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		},
	); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}

	var replicas int32
	if kcp.Spec.Replicas != nil {
		replicas = *kcp.Spec.Replicas
	}

	owner := metav1.OwnerReference{
		APIVersion:         ctrlv1beta1.GroupVersion.String(),
		Kind:               "KinkControlPlane",
		Name:               kcp.Name,
		UID:                kcp.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}

	for i := len(kms.Items); int32(i) < replicas; i++ {
		m := infrav1beta1.KinkMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.SimpleNameGenerator.GenerateName(cluster.Name + "-"),
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName:             cluster.Name,
					clusterv1.MachineControlPlaneLabelName: "",
				},
				OwnerReferences: []metav1.OwnerReference{owner},
			},
			Spec: infrav1beta1.KinkMachineSpec{
				Version: kcp.Spec.Version,
			},
		}
		if err := r.Create(ctx, &m); err != nil {
			logger.Error(err, "Filed to create KinkMachine for KinkControlPlane", "KinkControlPlane", kcp)
			continue
		}
	}

	for i := len(kms.Items); int32(i) > replicas; i-- {
		if err := r.Delete(ctx, &kms.Items[i-1]); err != nil {
			logger.Error(err, "Filed to delete KinkMachine from KinkControlPlane", "KinkControlPlane", kcp)
			continue
		}
	}

	// if KinkMachineList does not match replica, refresh it from apiserver.
	if len(kms.Items) != int(replicas) {
		kms = &infrav1beta1.KinkMachineList{}
		if err := r.Client.List(ctx, kms,
			client.InNamespace(cluster.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterLabelName: cluster.Name,
			},
		); err != nil {
			return nil, errors.Wrap(err, "failed to list machines")
		}
	}

	return kms, nil
}

func (r *KinkControlPlaneReconciler) updateKinkCtlPlaneStatus(ctx context.Context, kcp *ctrlv1beta1.KinkControlPlane, kms *infrav1beta1.KinkMachineList) error {
	var readyReplicas, unavailableReplicas int32
	for _, m := range kms.Items {
		if m.Status.Ready {
			readyReplicas++
		} else {
			unavailableReplicas++
		}
	}

	// Step 4: update KinkControlPlane's status accordingly
	kcp.Status.ReadyReplicas = readyReplicas
	kcp.Status.UnavailableReplicas = unavailableReplicas

	if kcp.Status.ReadyReplicas > 0 {
		kcp.Status.Initialized = true
		kcp.Status.Ready = true
	} else {
		kcp.Status.Initialized = false
		kcp.Status.Ready = false
	}

	if err := r.Status().Update(ctx, kcp); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KinkControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ctrlv1beta1.KinkControlPlane{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKinkCtrlPlane)).
		Owns(&infrav1beta1.KinkMachine{}).
		Owns(&v1.ConfigMap{}).
		Owns(&v1.Secret{}).
		Owns(&v1.Service{}).
		Complete(r)
}

func (r *KinkControlPlaneReconciler) ClusterToKinkCtrlPlane(o client.Object) []reconcile.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "KinkControlPlane" {
		return []ctrl.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: controlPlaneRef.Namespace,
					Name:      controlPlaneRef.Name,
				},
			},
		}
	}
	return nil
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

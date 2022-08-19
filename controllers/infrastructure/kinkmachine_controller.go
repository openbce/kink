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

package infrastructure

import (
	"context"
	"fmt"
	"openbce.io/kink/controllers/infrastructure/templates"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1beta1 "openbce.io/kink/apis/infrastructure/v1beta1"
	kinkutil "openbce.io/kink/controllers/util"
)

// KinkMachineReconciler reconciles a KinkMachine object
type KinkMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kinkmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kinkmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kinkmachines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KinkMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	machine := &infrav1beta1.KinkMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, machine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	cluster := &clusterv1.Cluster{}
	clusterName := types.NamespacedName{
		Namespace: machine.Namespace,
		Name:      machine.Labels[clusterv1.ClusterLabelName],
	}
	if err := r.Get(ctx, clusterName, cluster); err != nil {
		logger.Error(err, "Failed to get cluster for KinkMachine", "KinkMachine", machine)
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.lookupOrSetupControlPlane(ctx, cluster, machine); err != nil {
		logger.Error(err, "Failed to setup pods for KinkMachine", "KinkMachine", machine)
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.updateMachineStatus(ctx, cluster, machine); err != nil {
		logger.Error(err, "Failed to update the status of KinkMachine", "KinkMachine", machine)
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KinkMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta1.KinkMachine{}).
		Complete(r)
}

func (r *KinkMachineReconciler) lookupOrSetupPods(ctx context.Context, cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) error {
	podList := &v1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		}); err != nil {
		return err
	}

	machine.Status.Pods = nil

	podMap := map[infrav1beta1.ControlPlaneRole]*v1.Pod{}

	for _, pod := range podList.Items {
		if !util.IsOwnedByObject(&pod, machine) {
			continue
		}

		podType, err := kinkutil.GetControlPlaneRole(&pod.ObjectMeta)
		if err != nil {
			continue
		}

		podMap[podType] = &pod
	}

	podTemplates := r.getControlPlanePodTemplates(cluster, machine)

	for t, pt := range podTemplates {
		if pod, found := podMap[t]; found {
			podRef := v1.ObjectReference{
				Name:       pod.Name,
				Namespace:  pod.Namespace,
				UID:        pod.UID,
				APIVersion: "",
				Kind:       "Pod",
			}

			machine.Status.Pods = append(machine.Status.Pods, podRef)

			continue
		}

		if err := r.Create(ctx, pt); err != nil {
			continue
		}
	}

	return nil
}

func (r *KinkMachineReconciler) lookupOrSetupServices(ctx context.Context, cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) error {
	svcList := &v1.ServiceList{}
	if err := r.List(ctx, svcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		}); err != nil {
		return err
	}

	machine.Status.Pods = nil

	svcMap := map[infrav1beta1.ControlPlaneRole]*v1.Service{}

	for _, svc := range svcList.Items {
		if !util.IsOwnedByObject(&svc, machine) {
			continue
		}

		svcRole, err := kinkutil.GetControlPlaneRole(&svc.ObjectMeta)
		if err != nil {
			continue
		}

		svcMap[svcRole] = &svc
	}

	svcTemplates := r.getControlPlaneServiceTemplates(cluster, machine)

	for t, st := range svcTemplates {
		if _, found := svcMap[t]; found {
			continue
		}

		if err := r.Create(ctx, st); err != nil {
			continue
		}
	}

	return nil
}

func (r *KinkMachineReconciler) getControlPlanePodTemplates(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) map[infrav1beta1.ControlPlaneRole]*v1.Pod {
	res := map[infrav1beta1.ControlPlaneRole]*v1.Pod{}

	res[infrav1beta1.ETCD] = templates.EtcdPodTemplate(cluster, machine)
	res[infrav1beta1.ApiServer] = templates.ApiServerPodTemplate(cluster, machine)

	return res
}

func (r *KinkMachineReconciler) getControlPlaneServiceTemplates(cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) map[infrav1beta1.ControlPlaneRole]*v1.Service {
	res := map[infrav1beta1.ControlPlaneRole]*v1.Service{}

	res[infrav1beta1.ETCD] = templates.EtcdServiceTemplate(cluster, machine)

	return res
}

func (r *KinkMachineReconciler) updateMachineStatus(ctx context.Context, cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) error {
	podList := &v1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		}); err != nil {
		return err
	}

	machine.Status.Ready = true
	machine.Status.FailureMessage = nil
	machine.Status.FailureReason = nil

	for _, pod := range podList.Items {
		if !util.IsOwnedByObject(&pod, machine) {
			continue
		}

		if pod.Status.Phase != v1.PodRunning {
			reason := fmt.Sprintf("Pod %s/%s is not running", pod.Namespace, pod.Name)
			machine.Status.Ready = false
			machine.Status.FailureMessage = &reason
			machine.Status.FailureReason = &reason

			break
		}
	}

	if err := r.Update(ctx, machine); err != nil {
		return err
	}

	return nil
}

func (r *KinkMachineReconciler) lookupOrSetupControlPlane(ctx context.Context, cluster *clusterv1.Cluster, machine *infrav1beta1.KinkMachine) error {
	if err := r.lookupOrSetupPods(ctx, cluster, machine); err != nil {
		return err
	}

	if err := r.lookupOrSetupServices(ctx, cluster, machine); err != nil {
		return err
	}

	return nil
}

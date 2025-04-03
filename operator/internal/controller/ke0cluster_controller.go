/*
Copyright 2025.

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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "github.com/phonghmnguyen/ke0/operator/api/v1"
)

// Ke0ClusterReconciler reconciles a Ke0Cluster object
type Ke0ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cluster.ke0.com,resources=ke0clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.ke0.com,resources=ke0clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.ke0.com,resources=ke0clusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ke0Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *Ke0ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling Ke0Cluster CRs")

	ke0Cluster := &clusterv1.Ke0Cluster{}
	err := r.Get(ctx, req.NamespacedName, ke0Cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Ke0Cluster CR with this NamespacedName can not be found. This must be a DELETE event ")
			return ctrl.Result{}, nil
		}

		log.Info("Failed to get Ke0Cluster CR. Requeue work item")
		return ctrl.Result{}, err
	}

	if !r.checkPrequisites() {
		log.Info("Prerequisites have not been fulfilled")
	}

	return ctrl.Result{}, nil
}

func (r *Ke0ClusterReconciler) checkPrequisites() bool {
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *Ke0ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Ke0Cluster{}).
		Named("ke0cluster").
		Complete(r)
}

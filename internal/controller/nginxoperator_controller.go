/*
Copyright 2023.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/example/nginx-operator/api/v1alpha1"
	assets "github.com/example/nginx-operator/assets"
)

// NginxOperatorReconciler reconciles a NginxOperator object
type NginxOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NginxOperator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *NginxOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the NginxOperator resource
	operatorCR := &operatorv1alpha1.NginxOperator{}
	err := r.Get(ctx, req.NamespacedName, operatorCR)
	if err != nil {
		if errors.IsNotFound(err) {
			// Operator resource object not found
			logger.Info("Operator resource object not found")
			return ctrl.Result{}, nil
		}
		// Error getting operator resource object
		logger.Error(err, "Error getting operator resource object")
		return ctrl.Result{}, err
	}

	// Retrieve or create the Nginx deployment
	deployment := &appsv1.Deployment{}
	create := false
	err = r.Get(ctx, req.NamespacedName, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the deployment from file if not found
			create = true
			deployment = assets.GetDeploymentFromFile("manifests/nginx_deployment.yaml")
		} else {
			// Error getting existing Nginx deployment
			logger.Error(err, "Error getting existing Nginx deployment")
			return ctrl.Result{}, err
		}
	}

	// Set deployment fields
	deployment.Namespace = req.Namespace
	deployment.Name = req.Name
	replicas := int32(5)
	deployment.Spec.Replicas = pointer.Int32(replicas)
	// if operatorCR.Spec.Replicas != nil {
	// 	deployment.Spec.Replicas = operatorCR.Spec.Replicas
	// }
	if operatorCR.Spec.Port != nil {
		deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = *operatorCR.Spec.Port
	}
	ctrl.SetControllerReference(operatorCR, deployment, r.Scheme)

	// Create or update the deployment
	if create {
		err = r.Create(ctx, deployment)
	} else {
		err = r.Update(ctx, deployment)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// List pods associated with the deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
	}

	if err := r.List(ctx, podList, listOpts...); err != nil {
		logger.Error(err, "Error listing pods")
		return ctrl.Result{}, err
	}

	// Update the NginxOperatorStatus.Nodes with the pod names
	// Update the NginxOperatorStatus.Nodes with the names of active pods
	activePods := make([]string, 0)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			activePods = append(activePods, pod.Name)
		}
	}
	operatorCR.Status.Nodes = activePods

	// Update the NginxOperator status
	if err := r.Status().Update(ctx, operatorCR); err != nil {
		logger.Error(err, "Error updating NginxOperator status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NginxOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.NginxOperator{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

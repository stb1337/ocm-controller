/*
Copyright 2022.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	actionv1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmcontrollerv1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
)

// ComponentVersionReconciler reconciles a ComponentVersion object
type ComponentVersionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *ComponentVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("ocm-component-version-reconcile")
	log.Info("starting ocm component loop")

	component := &actionv1.ComponentVersion{}
	if err := r.Client.Get(ctx, req.NamespacedName, component); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, fmt.Errorf("failed to get component object: %w", err)
	}
	log.V(4).Info("found component", "component", component)
	//
	//session := ocm.NewSession(nil)
	//defer session.Close()
	//
	//ocmCtx := ocm.ForContext(ctx)
	//// configure credentials
	//if err := csdk.ConfigureCredentials(ctx, ocmCtx, r.Client, component.Spec.Repository.URL, component.Spec.Repository.SecretRef.Name, component.Namespace); err != nil {
	//	log.V(4).Error(err, "failed to find credentials")
	//	// ignore not found errors for now
	//	if !apierrors.IsNotFound(err) {
	//		return ctrl.Result{
	//			RequeueAfter: component.Spec.Interval,
	//		}, fmt.Errorf("failed to configure credentials for component: %w", err)
	//	}
	//}
	//
	//// get component version
	//cv, err := csdk.GetComponentVersion(ocmCtx, session, component.Spec.Repository.URL, component.Spec.Name, component.Spec.Version)
	//if err != nil {
	//	return ctrl.Result{
	//		RequeueAfter: component.Spec.Interval,
	//	}, err
	//}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ocmcontrollerv1.ComponentVersion{}).
		Complete(r)
}
/*
Copyright 2022 mobfun.

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mobfunv1 "vm-operator/api/v1"
)

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=mobfun.infinitefun.cn,resources=webapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mobfun.infinitefun.cn,resources=webapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mobfun.infinitefun.cn,resources=webapps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WebApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile

func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	k8slog := log.FromContext(ctx)

	// TODO(user): your logic here
	k8slog.Info("开始监听资源变化情况。")
	var webapp mobfunv1.WebApp
	err := r.Get(ctx, req.NamespacedName, &webapp)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	service := &corev1.Service{}

	err = r.Get(ctx, req.NamespacedName, service)

	if err != nil {
		k8slog.Info("service not exists")
		if err := updataSpecAnnotation(&webapp, ctx, r); err != nil {
			return ctrl.Result{}, err
		}
	}

	err = createDeployment(ctx, r, &webapp, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = createService(ctx, r, &webapp, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = createIngess(ctx, r, &webapp, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := updataSpecAnnotation(&webapp, ctx, r); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mobfunv1.WebApp{}).
		Complete(r)
}

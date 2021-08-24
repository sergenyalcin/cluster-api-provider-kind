/*
Copyright 2021.

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

	"github.com/go-logr/logr"
	infrastructurev1alpha1 "github.com/sergenyalcin/cluster-api-provider-kind/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/kind/pkg/cluster"
)

var finalizerName = "kindclusters.infrastructure.cluster-k8s.io/cluster-finalizer"

// KINDClusterReconciler reconciles a KINDCluster object
type KINDClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=infrastructure.cluster-k8s.io,resources=kindclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster-k8s.io,resources=kindclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster-k8s.io,resources=kindclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KINDCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *KINDClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = ctrl.Log.WithValues(infrastructurev1alpha1.KindOfKindCluster, req.NamespacedName)
	ctx = context.Background()

	var kindcluster infrastructurev1alpha1.KINDCluster

	r.Log.Info("Reconciling")

	if err := r.Client.Get(ctx, req.NamespacedName, &kindcluster); err != nil {
		if !k8serrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch KINDCluster instance")

			return ctrl.Result{}, err
		}

		r.Log.Info("KINDCluster resources cannot be found")

		return ctrl.Result{}, nil
	}

	provider := cluster.NewProvider()
	clusterList, err := provider.List()

	if err != nil {
		r.Log.Error(err, "unable to fetch clusters")

		return ctrl.Result{}, err
	}

	clusterName := kindcluster.Spec.ClusterName

	if kindcluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(finalizerName, kindcluster.GetFinalizers()) {
			controllerutil.AddFinalizer(&kindcluster, finalizerName)

			if err := r.Update(ctx, &kindcluster); err != nil {
				r.Log.Error(err, "unable to add finalizer")

				return ctrl.Result{}, err
			}

			r.Log.Info("Finalizer successfully added")

			return ctrl.Result{}, nil
		}
	} else {
		if containsString(finalizerName, kindcluster.GetFinalizers()) {
			if err := r.deleteResources(provider, clusterName, req.Namespace); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(&kindcluster, finalizerName)

			if err := r.Client.Update(ctx, &kindcluster); err != nil {
				r.Log.Error(err, "unable to update KINDCluster")

				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if containsString(clusterName, clusterList) {
		r.Log.Info("Specified cluster exists", "clustername", clusterName)

		kindcluster.Status.Ready = true
	} else {
		r.Log.Info("Specified cluster does not exist, will be created...", "clustername", clusterName)

		if err := provider.Create(clusterName, cluster.CreateWithKubeconfigPath(fmt.Sprintf("/tmp/%s_config", clusterName))); err != nil {
			r.Log.Error(err, "unable to create cluster")

			return ctrl.Result{}, err
		}

		kindcluster.Status.Conditions = append(kindcluster.Status.Conditions,
			infrastructurev1alpha1.KindClusterCondition{
				Timestamp: metav1.Now(),
				Message:   "Cluster was successfully created",
			})

		r.Log.Info("Specified cluster was successfully created", "clustername:", clusterName)
	}

	if err := r.Client.Status().Update(ctx, &kindcluster); err != nil {
		r.Log.Error(err, "unable to update KINDCluster status")

		return ctrl.Result{}, err
	}

	r.Log.Info("KINDCluster status was updated", "clustername:", clusterName)

	if err := storeKubeconfigInSecret(r.Client, provider, clusterName,
		fmt.Sprintf("%s-%s", clusterName, "config"), req.Namespace, r.Log); err != nil {

		r.Log.Error(err, "unable to store kubeconfig")

		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled")

	return ctrl.Result{}, nil
}

func containsString(s string, slice []string) bool {
	for _, finalizer := range slice {
		if finalizer == s {
			return true
		}
	}

	return false
}

func (r *KINDClusterReconciler) deleteResources(provider *cluster.Provider, clusterName, namespace string) error {
	r.Log.Info("Cluster is deleting...", "clustername", clusterName)

	if err := provider.Delete(clusterName, ""); err != nil {
		r.Log.Error(err, "unable to delete cluster")

		return err
	}

	r.Log.Info("Cluster successfully deleted", "clustername", clusterName)

	r.Log.Info("Config secret is deleting...", "clustername", clusterName)

	if err := r.Client.Delete(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-%s", clusterName, "config"),
		Namespace: namespace,
	}}); err != nil {
		if !k8serrors.IsNotFound(err) {
			r.Log.Error(err, "unable to delete kubeconfig secret of cluster")

			return err
		}
	}

	r.Log.Info("Config secret successfully deleted", "clustername", clusterName)

	return nil
}

func storeKubeconfigInSecret(c client.Client, provider *cluster.Provider, clusterName, secretName, namespace string, log logr.Logger) error {
	kubeconfigSecret := &corev1.Secret{}

	if err := c.Get(context.Background(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}, kubeconfigSecret); err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}

		kubeconfigBody, err := ioutil.ReadFile(fmt.Sprintf("/tmp/%s_config", clusterName))

		if err != nil {
			return err

		}

		kubeconfigSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"kubeconfig": kubeconfigBody,
			},
		}

		if err := c.Create(context.Background(), kubeconfigSecret); err != nil {
			return err
		}

		log.Info(fmt.Sprintf("%s config secret successfully created", secretName))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KINDClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.KINDCluster{}).
		Complete(r)
}

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

const (
	finalizerName = "kindclusters.infrastructure.cluster-k8s.io/cluster-finalizer"

	// Keys for logs
	clusterNameKey = "clusterName"
	secretNameKey  = "secretName"

	// Kubeconfig output from the kind tool is stored in a temporary file
	// This constant represents the template path of the temporary config file
	configFilePathTemplate = "/tmp/%s-config"
)

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
func (r *KINDClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Define the Logger instance of the Reconciler by using the request and the name of Reconciler
	r.Log = ctrl.Log.WithValues(infrastructurev1alpha1.KindOfKindCluster, req.NamespacedName)
	ctx = context.Background()

	// Reconciliation starts
	r.Log.Info("Reconciling")

	// Try to read the KINDCluster instance
	var kindcluster infrastructurev1alpha1.KINDCluster

	if err := r.Client.Get(ctx, req.NamespacedName, &kindcluster); err != nil {
		// If the error type is not "IsNotFound", then return error
		if !k8serrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch KINDCluster instance")

			return ctrl.Result{}, err
		}

		// If the error type is "IsNotFound", log the information and do not return error
		r.Log.Info("KINDCluster resources cannot be found")

		return ctrl.Result{}, nil
	}

	// Create a provider object to create, delete, list the clusters
	provider := cluster.NewProvider()
	clusterList, err := provider.List()

	if err != nil {
		r.Log.Error(err, "unable to fetch clusters")

		return ctrl.Result{}, err
	}

	// Read the cluster name from the spec of KINDCluster instance
	clusterName := kindcluster.Spec.ClusterName

	// Check DeletionTimestamp to decide if object is in deletion
	if kindcluster.ObjectMeta.DeletionTimestamp.IsZero() {
		// Object is not in deletion, so try to add the finalizer if it does not have the finalizer
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
		// Object is in deletion, so check the finalizer and delete the related resources, cluster and config secret
		if containsString(finalizerName, kindcluster.GetFinalizers()) {
			if err := deleteCluster(provider, clusterName, r.Log); err != nil {
				return ctrl.Result{}, err
			}

			if err := deleteConfigSecret(r.Client, r.Log, clusterName, req.Namespace); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(&kindcluster, finalizerName)

			if err := r.Client.Update(ctx, &kindcluster); err != nil {
				r.Log.Error(err, "unable to update KINDCluster")

				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	var creationError error

	// Check if the specified cluster exists
	if containsString(clusterName, clusterList) {
		// Cluster exists
		r.Log.Info("Specified cluster exists", clusterNameKey, clusterName)

		trueBool := true

		// Set the failureMessage to empty string and the ready bool to true
		kindcluster.Status.FailureMessage = ""
		kindcluster.Status.Ready = &trueBool
	} else {
		// Cluster does not exist
		r.Log.Info("Specified cluster does not exist, will be created...", clusterNameKey, clusterName)

		// Create the kind cluster
		if creationError = provider.Create(clusterName, cluster.CreateWithKubeconfigPath(getConfigFilePath(clusterName))); creationError != nil {
			r.Log.Error(creationError, "unable to create cluster")

			falseBool := false

			// If an issue occurs while creation, then add a status condition
			kindcluster.Status.Conditions = append(kindcluster.Status.Conditions,
				infrastructurev1alpha1.KindClusterCondition{
					Timestamp: metav1.Now(),
					Message:   "Cluster cannot be created",
					Reason:    creationError.Error(),
				})

			// If an issue occurs while creation, set the failure message and the ready bool to false
			kindcluster.Status.FailureMessage = fmt.Sprintf("Cluster cannot be crated: %s", creationError)
			kindcluster.Status.Ready = &falseBool
		} else {
			// If cluster was successfully created, then add a status condition
			kindcluster.Status.Conditions = append(kindcluster.Status.Conditions,
				infrastructurev1alpha1.KindClusterCondition{
					Timestamp: metav1.Now(),
					Message:   "Cluster was successfully created",
				})

			r.Log.Info("Specified cluster was successfully created", clusterNameKey, clusterName)
		}
	}

	// Update status of KINDCluster
	if err := r.Client.Status().Update(ctx, &kindcluster); err != nil {
		r.Log.Error(err, "unable to update KINDCluster status")

		return ctrl.Result{}, err
	}

	r.Log.Info("KINDCluster status was updated", clusterNameKey, clusterName)

	// If an error occured while the creation of cluster, return the error after the status subresource was updated
	if creationError != nil {
		return ctrl.Result{}, creationError
	}

	// Store the kubeconfig in  a secret
	if err := storeKubeconfigInSecret(r.Client, clusterName,
		getConfigSecretName(clusterName), req.Namespace, r.Log); err != nil {

		r.Log.Error(err, "unable to store kubeconfig")

		return ctrl.Result{}, err
	}

	// Reconciliation finishes
	r.Log.Info("Reconciled")

	return ctrl.Result{}, nil
}

// Check whether a slice contains a specified string
func containsString(s string, slice []string) bool {
	for _, finalizer := range slice {
		if finalizer == s {
			return true
		}
	}

	return false
}

// Get the secret name from the clustername
func getConfigSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, "config")
}

// Get the kubeconfig file path from the clustername
func getConfigFilePath(clusterName string) string {
	return fmt.Sprintf(configFilePathTemplate, clusterName)
}

// Delete the external resources: kind cluster
func deleteCluster(provider *cluster.Provider, clusterName string, log logr.Logger) error {
	log.Info("Cluster is deleting...", clusterNameKey, clusterName)

	// Delete the kind cluster
	// No check has been done as to whether the cluster already exists.
	// Because the kind tool is idempotent and it does not return an error when it cannot find the cluster.
	if err := provider.Delete(clusterName, ""); err != nil {
		log.Error(err, "unable to delete cluster")

		return err
	}

	log.Info("Cluster successfully deleted", clusterNameKey, clusterName)

	return nil
}

// Delete the external resources: config secret
func deleteConfigSecret(c client.Client, log logr.Logger, clusterName, namespace string) error {
	log.Info("Config secret is deleting...", clusterNameKey, clusterName)

	// Delete the config secret
	// If it does not exist, ignore the deletion
	if err := c.Delete(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      getConfigSecretName(clusterName),
		Namespace: namespace,
	}}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "unable to delete kubeconfig secret of cluster")

			return err
		}
	}

	log.Info("Config secret successfully deleted", clusterNameKey, clusterName)

	return nil
}

// Store the kubeconfig of cluster in a secret
func storeKubeconfigInSecret(c client.Client, clusterName, secretName, namespace string, log logr.Logger) error {
	kubeconfigSecret := &corev1.Secret{}

	// Try to get the config secret
	if err := c.Get(context.Background(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}, kubeconfigSecret); err != nil {

		// If the error type is not "IsNotFound", then return error
		if !k8serrors.IsNotFound(err) {
			return err
		}

		// If the error type is "IsNotFound", this means that the config secret has not been created
		// Start to create the config secret

		// Read the kubeconfig from the temporary file
		kubeconfigBody, err := ioutil.ReadFile(getConfigFilePath(clusterName))

		if err != nil {
			return err
		}

		// Create the secret object
		kubeconfigSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"config": kubeconfigBody,
			},
		}

		// Create the real secret object
		if err := c.Create(context.Background(), kubeconfigSecret); err != nil {
			return err
		}

		log.Info("Config secret successfully created", secretNameKey, secretName, clusterNameKey, clusterName)
	}

	// If err is nil, this means that the config secret was successfully created earlier, so do nothing and return nil
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KINDClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch the KINDCluster instances to trigger the reconciler
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.KINDCluster{}).
		Complete(r)
}

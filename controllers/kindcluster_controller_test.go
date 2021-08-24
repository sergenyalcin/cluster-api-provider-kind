package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	infrastructurev1alpha1 "github.com/sergenyalcin/cluster-api-provider-kind/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_ContainsString(t *testing.T) {
	var testCases = []struct {
		slice  []string
		s      string
		result bool
	}{
		{[]string{"testcluster-1", "testcluster-2"}, "testcluster", false},
		{[]string{"testcluster-1", "testcluster-2", "testcluster-3"}, "testcluster-2", true},
	}
	for _, tc := range testCases {
		t.Run(tc.s, func(t *testing.T) {
			if got := containsString(tc.s, tc.slice); got != tc.result {
				t.Errorf("containsString() = %v, want %v", got, tc.result)
			}
		})
	}
}

func Test_GetConfigSecretName(t *testing.T) {
	var testCases = []struct {
		clusterName string
		secretName  string
	}{
		{"test-cluster", "test-cluster-config"},
		{"", "-config"},
	}
	for _, tc := range testCases {
		t.Run(tc.clusterName, func(t *testing.T) {
			if got := getConfigSecretName(tc.clusterName); got != tc.secretName {
				t.Errorf("containsString() = %v, want %v", got, tc.secretName)
			}
		})
	}
}

func Test_GetConfigFilePath(t *testing.T) {
	var testCases = []struct {
		clusterName string
		filePath    string
	}{
		{"test-cluster", "/tmp/test-cluster-config"},
		{"", "/tmp/-config"},
	}
	for _, tc := range testCases {
		t.Run(tc.clusterName, func(t *testing.T) {
			if got := getConfigFilePath(tc.clusterName); got != tc.filePath {
				t.Errorf("containsString() = %v, want %v", got, tc.filePath)
			}
		})
	}
}

func Test_StoreKubeconfigInSecret(t *testing.T) {
	infrastructurev1alpha1.AddToScheme(scheme.Scheme)

	c := fake.NewFakeClientWithScheme(scheme.Scheme, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	})

	resultSecret := corev1.Secret{
		Data: map[string][]byte{
			"config": []byte("kubeconfigData"),
		},
	}

	log := ctrl.Log.WithValues(infrastructurev1alpha1.KindOfKindCluster)

	var testCases = []struct {
		clusterName string
		secretName  string
		namespace   string
		result      corev1.Secret
	}{
		{"test", "test-config", "default", resultSecret},
	}
	for _, tc := range testCases {
		t.Run(tc.clusterName, func(t *testing.T) {
			err := ioutil.WriteFile(fmt.Sprintf(getConfigFilePath(tc.clusterName)), []byte("kubeconfigData"), 0755)

			if err != nil {
				fmt.Printf("Unable to write file: %v", err)
			}

			defer func() {
				err := os.Remove(getConfigFilePath(tc.clusterName))

				if err != nil {
					panic(err)
				}
			}()

			if err := storeKubeconfigInSecret(c, tc.clusterName, tc.secretName, tc.namespace, log); err != nil {
				panic(err)
			}

			secret := &corev1.Secret{}
			if err := c.Get(context.Background(), types.NamespacedName{Name: tc.secretName, Namespace: tc.namespace}, secret); err != nil {
				panic(err)
			}

			if string(tc.result.Data["config"]) != string(secret.Data["config"]) {
				t.Errorf("storeKubeconfigInSecret() = %v, want %v", tc.result.Data, secret.Data)
			}
		})
	}
}

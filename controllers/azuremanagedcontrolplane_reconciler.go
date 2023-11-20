/*
Copyright 2019 The Kubernetes Authors.

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

	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2022-03-01/containerservice"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/scope"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/groups"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedclusters"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedroleassignments"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedroledefinitions"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/privateendpoints"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/resourcehealth"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/subnets"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/tags"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/virtualnetworks"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// azureManagedControlPlaneService contains the services required by the cluster controller.
type azureManagedControlPlaneService struct {
	kubeclient client.Client
	scope      managedclusters.ManagedClusterScope
	services   []azure.ServiceReconciler
}

// newAzureManagedControlPlaneReconciler populates all the services based on input scope.
func newAzureManagedControlPlaneReconciler(scope *scope.ManagedControlPlaneScope) *azureManagedControlPlaneService {
	return &azureManagedControlPlaneService{
		kubeclient: scope.Client,
		scope:      scope,
		services: []azure.ServiceReconciler{
			groups.New(scope),
			virtualnetworks.New(scope),
			subnets.New(scope),
			managedclusters.New(scope),
			privateendpoints.New(scope),
			tags.New(scope),
			resourcehealth.New(scope),
			managedroledefinitions.New(scope),
			managedroleassignments.New(scope),
		},
	}
}

// Reconcile reconciles all the services in a predetermined order.
func (r *azureManagedControlPlaneService) Reconcile(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.Reconcile")
	defer done()

	for _, service := range r.services {
		if err := service.Reconcile(ctx); err != nil {
			return errors.Wrapf(err, "failed to reconcile AzureManagedControlPlane service %s", service.Name())
		}
	}
	if err := r.reconcileKubeconfig(ctx); err != nil {
		return errors.Wrap(err, "failed to reconcile kubeconfig secret")
	}

	if err := r.deleteUnmanagedAgentPools(ctx); err != nil {
		return errors.Wrap(err, "failed to delete unmanaged agent pools")
	}

	return nil
}

func (r *azureManagedControlPlaneService) deleteUnmanagedAgentPools(ctx context.Context) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.deleteUnmanagedAgentPools")
	defer done()
	clusterName := r.scope.ManagedClusterSpec().ResourceName()
	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{clusterv1.ClusterNameLabel: clusterName}),
	}
	managedMachinePools := &infrav1.AzureManagedMachinePoolList{}
	if err := r.kubeclient.List(ctx, managedMachinePools, listOptions...); err != nil {
		return fmt.Errorf("failed to list managed machine pools for cluster %s: %w", clusterName, err)
	}
	poolMap := make(map[string]struct{})
	for _, pool := range managedMachinePools.Items {
		poolMap[*pool.Spec.Name] = struct{}{}
	}

	agentPoolsClient := containerservice.NewAgentPoolsClientWithBaseURI(r.scope.BaseURI(), r.scope.SubscriptionID())
	azure.SetAutoRestClientDefaults(&agentPoolsClient.Client, r.scope.Authorizer())

	result, err := agentPoolsClient.List(ctx, r.scope.ManagedClusterSpec().ResourceGroupName(), clusterName)
	if err != nil {
		return errors.Wrap(err, "failed to list agent pools")
	}
	for result.NotDone() {
		for _, pool := range result.Values() {
			if _, ok := poolMap[*pool.Name]; ok {
				continue
			}
			log.Info("start delete node group", "nodeGroupName", pool, "clusterName", clusterName)
			if _, err = agentPoolsClient.Delete(ctx, r.scope.ManagedClusterSpec().ResourceGroupName(), clusterName, *pool.Name); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to delete agent pool %s in cluster %s", *pool.Name, clusterName))
			}
		}
		if err = result.NextWithContext(ctx); err != nil {
			return errors.Wrap(err, "failed to list agent pools")
		}
	}
	return nil
}

// Delete reconciles all the services in a predetermined order.
func (r *azureManagedControlPlaneService) Delete(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.Delete")
	defer done()

	// Delete services in reverse order of creation.
	for i := len(r.services) - 1; i >= 0; i-- {
		if err := r.services[i].Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete AzureManagedControlPlane service %s", r.services[i].Name())
		}
	}

	return nil
}

func (r *azureManagedControlPlaneService) reconcileKubeconfig(ctx context.Context) error {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "controllers.azureManagedControlPlaneService.reconcileKubeconfig")
	defer done()

	kubeConfigData := r.scope.GetKubeConfigData()
	if kubeConfigData == nil {
		return nil
	}
	kubeConfigSecret := r.scope.MakeEmptyKubeConfigSecret()

	// Always update credentials in case of rotation
	if _, err := controllerutil.CreateOrUpdate(ctx, r.kubeclient, &kubeConfigSecret, func() error {
		if kubeConfigSecret.Labels == nil {
			kubeConfigSecret.Labels = make(map[string]string)
		}
		kubeConfigSecret.Labels["cluster.x-k8s.io/cluster-name"] = r.scope.ManagedClusterSpec().ResourceName()

		kubeConfigSecret.Data = map[string][]byte{
			secret.KubeconfigDataName: kubeConfigData,
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to kubeconfig secret for cluster")
	}

	return nil
}

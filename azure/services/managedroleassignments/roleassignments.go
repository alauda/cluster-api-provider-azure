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

package managedroleassignments

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2022-03-01/containerservice"
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/async"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/managedclusters"
	"sigs.k8s.io/cluster-api-provider-azure/util/reconciler"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

const serviceName = "roleassignments"

// RoleAssignmentScope defines the scope interface for a role assignment service.
type RoleAssignmentScope interface {
	azure.AsyncStatusUpdater
	azure.Authorizer
	RoleAssignmentSpecs(principalID *string) []azure.ResourceSpecGetter
	ResourceGroup() string
	ClusterName() string
	BaseURI() string
	SubscriptionID() string
}

// Service provides operations on Azure resources.
type Service struct {
	Scope RoleAssignmentScope
	async.Reconciler
	managedClustersClient *managedclusters.AzureClient
}

// New creates a new service.
func New(scope RoleAssignmentScope) *Service {
	client := newClient(scope)
	return &Service{
		Scope:                 scope,
		managedClustersClient: managedclusters.NewClient(scope),
		Reconciler:            async.New(scope, client, client),
	}
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

// Reconcile idempotently creates or updates a role assignment.
func (s *Service) Reconcile(ctx context.Context) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "roleassignments.Service.Reconcile")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultAzureServiceReconcileTimeout)
	defer cancel()
	log.V(2).Info("reconciling role assignment")

	principalID, err := s.getManagedClusterPrincipalID(ctx)
	if err != nil {
		log.Error(err, "failed to get managed cluster principle ID", "cluster", s.Scope.ClusterName())
		return err
	}

	for _, roleAssignmentSpec := range s.Scope.RoleAssignmentSpecs(principalID) {
		log.V(2).Info("Creating role assignment")
		if roleAssignmentSpec.ResourceName() == "" {
			log.V(2).Info("RoleAssignmentName is empty. This is not expected and will cause this System Assigned Identity to have no permissions.")
		}
		_, err := s.CreateOrUpdateResource(ctx, roleAssignmentSpec, serviceName)
		if err != nil {
			log.Error(err, "failed to create role assignment", "assignment", roleAssignmentSpec.ResourceName())
			return errors.Wrapf(err, "cannot assign role to system assigned identity")
		}
	}

	return nil
}

func (s *Service) getManagedClusterPrincipalID(ctx context.Context) (*string, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "roleassignments.Service.getManagedClusterPrincipalID")
	defer done()
	log.V(2).Info("fetching principal ID for ManagedCluster")
	obj, err := s.managedClustersClient.Get(ctx, &managedclusters.ManagedClusterSpec{
		ResourceGroup: s.Scope.ResourceGroup(),
		Name:          s.Scope.ClusterName(),
	})
	if err != nil {
		return nil, err
	}
	cluster, ok := obj.(containerservice.ManagedCluster)
	if !ok {
		return nil, errors.Errorf("%T is not a containerservice.ManagedCluster", obj)
	}
	return cluster.Identity.PrincipalID, nil
}

// Delete is a no-op as the role assignments get deleted.
func (s *Service) Delete(ctx context.Context) error {
	_, log, done := tele.StartSpanWithLogger(ctx, "roleassignments.Service.Delete")
	defer done()
	for _, roleAssignmentSpec := range s.Scope.RoleAssignmentSpecs(nil) {
		log.V(2).Info("Deleting role assignment")
		if roleAssignmentSpec.ResourceName() == "" {
			log.V(2).Info("RoleAssignmentName is empty. This is not expected and will cause this System Assigned Identity to have no permissions.")
		}
		err := s.DeleteResource(ctx, roleAssignmentSpec, serviceName)
		if err != nil {
			log.Error(err, "failed to delete role assignment", "assignment", roleAssignmentSpec.ResourceName())
			return errors.Wrapf(err, "cannot delete assign role assigned")
		}
	}
	return nil
}

// IsManaged returns always returns true as CAPZ does not support BYO role assignments.
func (s *Service) IsManaged(ctx context.Context) (bool, error) {
	return true, nil
}

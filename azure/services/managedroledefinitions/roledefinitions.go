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

package managedroledefinitions

import (
	"context"

	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/async"
	"sigs.k8s.io/cluster-api-provider-azure/util/reconciler"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

const serviceName = "roledefinitions"

// RoleDefinitionScope defines the scope interface for a role definition service.
type RoleDefinitionScope interface {
	azure.AsyncStatusUpdater
	azure.Authorizer
	RoleDefinitionSpecs() []azure.ResourceSpecGetter
	ResourceGroup() string
}

// Service provides operations on Azure resources.
type Service struct {
	Scope RoleDefinitionScope
	async.Reconciler
}

// New creates a new service.
func New(scope RoleDefinitionScope) *Service {
	client := newClient(scope)
	return &Service{
		Scope:      scope,
		Reconciler: async.New(scope, client, client),
	}
}

// Name returns the service name.
func (s *Service) Name() string {
	return serviceName
}

// Reconcile idempotent creates or updates a role definition.
func (s *Service) Reconcile(ctx context.Context) error {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "roledefinitions.Service.Reconcile")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultAzureServiceReconcileTimeout)
	defer cancel()
	log.V(2).Info("reconciling role definition")

	for _, roleDefinitionSpec := range s.Scope.RoleDefinitionSpecs() {
		log.V(2).Info("Creating role definition")
		_, err := s.CreateOrUpdateResource(ctx, roleDefinitionSpec, serviceName)
		if err != nil {
			log.Error(err, "failed to create role definition", "definition", roleDefinitionSpec.ResourceName())
			return errors.Wrapf(err, "cannot create or update role definition %s", roleDefinitionSpec.ResourceName())
		}
	}
	return nil
}

// Delete is a no-op of role definitions.
// TODO: delete role definition util all the referenced role assignments are deleted.
func (s *Service) Delete(ctx context.Context) error {
	return nil
}

// IsManaged returns always returns true as CAPZ does not support BYO role definitions.
func (s *Service) IsManaged(ctx context.Context) (bool, error) {
	return true, nil
}

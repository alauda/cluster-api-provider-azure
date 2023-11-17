/*
Copyright 2020 The Kubernetes Authors.

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

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/authorization/mgmt/authorization"
	"github.com/Azure/go-autorest/autorest"
	azureautorest "github.com/Azure/go-autorest/autorest/azure"
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/util/tele"
)

// azureClient contains the Azure go-sdk Client.
type azureClient struct {
	client authorization.RoleDefinitionsClient
}

// newClient creates a new role definition client from subscription ID.
func newClient(auth azure.Authorizer) *azureClient {
	client := newRoleDefinitionClient(auth.SubscriptionID(), auth.BaseURI(), auth.Authorizer())
	return &azureClient{client}
}

// newRoleDefinitionClient creates a role definition client from subscription ID.
func newRoleDefinitionClient(subscriptionID string, baseURI string, authorizer autorest.Authorizer) authorization.RoleDefinitionsClient {
	client := authorization.NewRoleDefinitionsClientWithBaseURI(baseURI, subscriptionID)
	azure.SetAutoRestClientDefaults(&client.Client, authorizer)
	return client
}

// Get gets the specified role definition by the role definition ID.
func (ac *azureClient) Get(ctx context.Context, spec azure.ResourceSpecGetter) (interface{}, error) {
	ctx, span := tele.Tracer().Start(ctx, "definitions.AzureClient.Get")
	defer span.End()
	return ac.client.Get(ctx, spec.OwnerResourceName(), spec.ResourceName())
}

// CreateOrUpdateAsync creates a role definition.
// Creating a role definition is not a long running operation, so we don't ever return a future.
func (ac *azureClient) CreateOrUpdateAsync(ctx context.Context, spec azure.ResourceSpecGetter, parameters interface{}) (interface{}, azureautorest.FutureAPI, error) {
	ctx, log, done := tele.StartSpanWithLogger(ctx, "groups.AzureClient.CreateOrUpdate")
	defer done()
	createParams, ok := parameters.(authorization.RoleDefinitionProperties)
	if !ok {
		return nil, nil, errors.Errorf("%T is not a authorization.RoleDefinitionProperties", parameters)
	}
	roleDefinitionID := spec.ResourceName()
	result, err := ac.client.CreateOrUpdate(ctx, spec.OwnerResourceName(), roleDefinitionID, authorization.RoleDefinition{
		ID:                       &roleDefinitionID,
		Name:                     createParams.RoleName,
		RoleDefinitionProperties: &createParams,
	})
	if err != nil {
		log.Error(err, "create role definition failed")
	} else {
		log.Info("create role definition successfully", "id", result.ID, "name", result.Name, "result", result)
	}
	return result, nil, err
}

// IsDone returns true if the long-running operation has completed.
func (ac *azureClient) IsDone(ctx context.Context, future azureautorest.FutureAPI) (bool, error) {
	ctx, _, done := tele.StartSpanWithLogger(ctx, "roledefinitions.AzureClient.IsDone")
	defer done()

	return future.DoneWithContext(ctx, ac.client)
}

// Result fetches the result of a long-running operation future.
func (ac *azureClient) Result(ctx context.Context, futureData azureautorest.FutureAPI, futureType string) (interface{}, error) {
	// Result is a no-op for role definitions as only Delete operations return a future.
	return nil, nil
}

// DeleteAsync is no-op for role definitions.
func (ac *azureClient) DeleteAsync(ctx context.Context, spec azure.ResourceSpecGetter) (azureautorest.FutureAPI, error) {
	return nil, nil
}

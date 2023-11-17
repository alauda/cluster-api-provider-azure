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

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/authorization/mgmt/authorization"
)

// RoleDefinitionSpec defines the specification for a role definition.
type RoleDefinitionSpec struct {
	ResourceGroup    string
	RoleDefinitionID string
	Scope            string
	RoleName         *string
	AssignableScopes *[]string
	Permissions      *[]authorization.Permission
}

// ResourceName returns the name of the role definition.
func (s *RoleDefinitionSpec) ResourceName() string {
	return s.RoleDefinitionID
}

// ResourceGroupName returns the name of the resource group.
func (s *RoleDefinitionSpec) ResourceGroupName() string {
	return s.ResourceGroup
}

// OwnerResourceName returns the scope for role definition.
// TODO: Consider renaming the function for better readability (@sonasingh46).
func (s *RoleDefinitionSpec) OwnerResourceName() string {
	return s.Scope
}

// Parameters returns the parameters for the RoleDefinitionSpec.
func (s *RoleDefinitionSpec) Parameters(ctx context.Context, existing interface{}) (interface{}, error) {
	if existing != nil {
		if _, ok := existing.(authorization.RoleDefinition); !ok {
			return nil, errors.Errorf("%T is not a authorization.RoleDefinition", existing)
		}
		// RoleDefinitionSpec already exists
		return nil, nil
	}
	roleType := "CustomRole"
	return authorization.RoleDefinitionProperties{
		RoleName:         s.RoleName,
		AssignableScopes: s.AssignableScopes,
		Permissions:      s.Permissions,
		RoleType:         &roleType,
	}, nil
}

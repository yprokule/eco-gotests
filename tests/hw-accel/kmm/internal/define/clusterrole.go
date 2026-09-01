package define

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	rbacv1 "k8s.io/api/rbac/v1"
)

// DRADriverClusterRole returns a ClusterRole builder with resource.k8s.io RBAC for a DRA driver.
func DRADriverClusterRole(name string) rbac.ClusterRoleBuilder {
	clusterRole := rbac.NewClusterRoleBuilder(inittools.APIClient, name,
		rbacv1.PolicyRule{
			APIGroups: []string{"resource.k8s.io"},
			Resources: []string{"resourceslices"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		}).WithRules([]rbacv1.PolicyRule{
		{
			APIGroups: []string{"resource.k8s.io"},
			Resources: []string{"resourceclaims"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     []string{"get"},
		},
	})

	return *clusterRole
}

// DRADriverCRB returns a ClusterRoleBinding builder for a DRA driver ServiceAccount.
func DRADriverCRB(name, clusterRoleName, saName, saNamespace string) rbac.ClusterRoleBindingBuilder {
	crb := rbac.NewClusterRoleBindingBuilder(inittools.APIClient,
		name,
		clusterRoleName,
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: saNamespace,
		})

	return *crb
}

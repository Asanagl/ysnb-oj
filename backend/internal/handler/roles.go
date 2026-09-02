// Package handler — role helpers. super_admin (introduced with multi-admin
// management) outranks admin and inherits every admin power; these helpers
// are the single place that spelling is decided so no check site forgets.
package handler

import "github.com/ysnb/oj/internal/model"

// isAdminRole: super_admin and admin both pass admin-gated routes.
func isAdminRole(role string) bool {
	return role == model.RoleAdmin || role == model.RoleSuperAdmin
}

// isSetterRole: roles allowed to author/manage problems and contests.
func isSetterRole(role string) bool {
	return role == model.RoleSetter || isAdminRole(role)
}

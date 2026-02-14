package rbac

import "fmt"

// Role represents a user's access level.
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
)

// Action represents an operation a user wants to perform.
type Action string

const (
	ActionRead        Action = "read"
	ActionWrite       Action = "write"
	ActionDelete      Action = "delete"
	ActionManageUsers Action = "manage_users"
	ActionViewAudit   Action = "view_audit"
	ActionExport      Action = "export"
)

// permissions maps roles to their allowed actions.
var permissions = map[Role]map[Action]bool{
	RoleAdmin: {
		ActionRead:        true,
		ActionWrite:       true,
		ActionDelete:      true,
		ActionManageUsers: true,
		ActionViewAudit:   true,
		ActionExport:      true,
	},
	RoleDeveloper: {
		ActionRead:   true,
		ActionWrite:  true,
		ActionDelete: true,
	},
	RoleViewer: {
		ActionRead: true,
	},
}

// Can checks if a role is allowed to perform an action.
func Can(role Role, action Action) bool {
	rolePerms, ok := permissions[role]
	if !ok {
		return false
	}
	return rolePerms[action]
}

// ValidateRole checks if a string is a valid role.
func ValidateRole(s string) (Role, error) {
	switch Role(s) {
	case RoleAdmin, RoleDeveloper, RoleViewer:
		return Role(s), nil
	default:
		return "", fmt.Errorf("invalid role %q (must be admin, developer, or viewer)", s)
	}
}

// Enforce checks if a user with the given role can perform the action.
// Returns an error if not permitted.
func Enforce(role Role, action Action) error {
	if !Can(role, action) {
		return fmt.Errorf("permission denied: %s cannot %s", role, action)
	}
	return nil
}

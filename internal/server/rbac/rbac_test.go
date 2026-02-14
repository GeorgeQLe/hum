package rbac

import "testing"

func TestAdminPermissions(t *testing.T) {
	actions := []Action{ActionRead, ActionWrite, ActionDelete, ActionManageUsers, ActionViewAudit, ActionExport}
	for _, a := range actions {
		if !Can(RoleAdmin, a) {
			t.Errorf("admin should be allowed to %s", a)
		}
	}
}

func TestDeveloperPermissions(t *testing.T) {
	allowed := []Action{ActionRead, ActionWrite, ActionDelete}
	for _, a := range allowed {
		if !Can(RoleDeveloper, a) {
			t.Errorf("developer should be allowed to %s", a)
		}
	}

	denied := []Action{ActionManageUsers, ActionViewAudit, ActionExport}
	for _, a := range denied {
		if Can(RoleDeveloper, a) {
			t.Errorf("developer should NOT be allowed to %s", a)
		}
	}
}

func TestViewerPermissions(t *testing.T) {
	if !Can(RoleViewer, ActionRead) {
		t.Error("viewer should be allowed to read")
	}

	denied := []Action{ActionWrite, ActionDelete, ActionManageUsers, ActionViewAudit}
	for _, a := range denied {
		if Can(RoleViewer, a) {
			t.Errorf("viewer should NOT be allowed to %s", a)
		}
	}
}

func TestInvalidRole(t *testing.T) {
	if Can(Role("superadmin"), ActionRead) {
		t.Error("invalid role should not have any permissions")
	}
}

func TestValidateRole(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"admin", true},
		{"developer", true},
		{"viewer", true},
		{"superadmin", false},
		{"", false},
	}

	for _, tt := range tests {
		_, err := ValidateRole(tt.input)
		if (err == nil) != tt.valid {
			t.Errorf("ValidateRole(%q) valid=%v, want %v", tt.input, err == nil, tt.valid)
		}
	}
}

func TestEnforce(t *testing.T) {
	if err := Enforce(RoleAdmin, ActionManageUsers); err != nil {
		t.Errorf("admin should be allowed to manage users: %v", err)
	}

	if err := Enforce(RoleViewer, ActionWrite); err == nil {
		t.Error("viewer should not be allowed to write")
	}
}

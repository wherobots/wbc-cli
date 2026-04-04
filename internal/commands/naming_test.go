package commands

import (
	"reflect"
	"testing"

	"wherobots/cli/internal/spec"
)

func TestChooseVerbUsesFirstWordOfOperationID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		operationID string
		want        string
	}{
		{"fetchUsers", "fetch"},
		{"listJobRuns", "list"},
		{"getFeatureFlags", "get"},
		{"cancelRun", "cancel"},
		{"destroyNotebookInstance", "destroy"},
		{"setDefaultRegion", "set"},
		{"undeleteMyOrganization", "undelete"},
	}
	for _, tc := range cases {
		op := &spec.Operation{Method: "GET", Path: "/x", OperationID: tc.operationID}
		if got := ChooseVerb(op); got != tc.want {
			t.Fatalf("ChooseVerb(%q) = %q, want %q", tc.operationID, got, tc.want)
		}
	}
}

func TestChooseVerbFallsBackToHTTPHeuristic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		op   *spec.Operation
		want string
	}{
		{op: &spec.Operation{Method: "GET", Path: "/users"}, want: "list"},
		{op: &spec.Operation{Method: "GET", Path: "/users/{id}", PathParamOrder: []string{"id"}}, want: "get"},
		{op: &spec.Operation{Method: "POST", Path: "/users"}, want: "create"},
		{op: &spec.Operation{Method: "PATCH", Path: "/users/{id}"}, want: "update"},
	}

	for _, tc := range cases {
		if got := ChooseVerb(tc.op); got != tc.want {
			t.Fatalf("ChooseVerb(%s %s) = %q, want %q", tc.op.Method, tc.op.Path, got, tc.want)
		}
	}
}

func TestPathToResourceSegments(t *testing.T) {
	t.Parallel()

	got := PathToResourceSegments("/users/{id}/settings")
	want := []string{"users", "settings"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PathToResourceSegments() = %v, want %v", got, want)
	}
}

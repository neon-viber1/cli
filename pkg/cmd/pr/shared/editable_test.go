package shared

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/stretchr/testify/require"
)

func TestEditable_ProjectV2Ids_ownerNumber(t *testing.T) {
	metadata := api.RepoMetadataResult{ProjectsV2: []api.ProjectV2{
		{ID: "USERID", Title: "my-project", Number: 1, ResourcePath: "/users/monalisa/projects/1"},
		{ID: "ORGID", Title: "my-project", Number: 3, ResourcePath: "/orgs/acme/projects/3"},
	}}

	t.Run("add and remove by owner/number", func(t *testing.T) {
		e := Editable{
			Metadata: metadata,
			Projects: EditableProjects{EditableSlice: EditableSlice{
				Add:    []string{"acme/3"},
				Remove: []string{"monalisa/1"},
				Edited: true,
			}},
		}
		add, remove, err := e.ProjectV2Ids()
		require.NoError(t, err)
		require.Equal(t, []string{"ORGID"}, *add)
		require.Equal(t, []string{"USERID"}, *remove)
	})

	t.Run("ambiguous title is rejected", func(t *testing.T) {
		e := Editable{
			Metadata: metadata,
			Projects: EditableProjects{EditableSlice: EditableSlice{
				Add:    []string{"my-project"},
				Edited: true,
			}},
		}
		_, _, err := e.ProjectV2Ids()
		require.EqualError(t, err, "'my-project' matches multiple projects: monalisa/1, acme/3; use <owner>/<number> to choose one")
	})
}

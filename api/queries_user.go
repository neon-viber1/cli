package api

import (
	"errors"

	"github.com/shurcooL/githubv4"
)

type Organization struct {
	Login string
}

func CurrentLoginName(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			Login string
		}
	}
	err := client.Query(hostname, "UserCurrent", &query, nil)
	return query.Viewer.Login, err
}

func CurrentLoginNameAndOrgs(client *Client, hostname string) (string, []string, error) {
	var query struct {
		Viewer struct {
			Login         string
			Organizations struct {
				Nodes []Organization
			} `graphql:"organizations(first: 100)"`
		}
	}
	err := client.Query(hostname, "UserCurrent", &query, nil)
	if err != nil {
		return "", nil, err
	}
	orgNames := []string{}
	for _, org := range query.Viewer.Organizations.Nodes {
		orgNames = append(orgNames, org.Login)
	}
	return query.Viewer.Login, orgNames, nil
}

func CurrentUserID(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			ID string
		}
	}
	err := client.Query(hostname, "UserCurrent", &query, nil)
	return query.Viewer.ID, err
}

// UserID resolves a user login to its node ID. It returns an empty ID and no
// error when no such user exists.
func UserID(client *Client, hostname, login string) (string, error) {
	var query struct {
		User struct {
			ID string
		} `graphql:"user(login: $login)"`
	}
	variables := map[string]any{"login": githubv4.String(login)}
	err := client.Query(hostname, "UserID", &query, variables)
	if err != nil {
		if gqlErr, ok := errors.AsType[GraphQLError](err); ok && gqlErr.Match("NOT_FOUND", "user") {
			return "", nil
		}
		return "", err
	}
	return query.User.ID, nil
}

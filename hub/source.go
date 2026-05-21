package hub

import (
	"fmt"
	"strings"
)

/*
ParseLocator converts hf://model/org/repo@revision style strings into a
repo location. Plain repo identifiers are treated as model repos on main.
*/
func ParseLocator(source string) (RepoLocation, error) {
	source = strings.TrimSpace(source)

	if source == "" {
		return RepoLocation{}, fmt.Errorf("hub: source is required")
	}

	if !strings.HasPrefix(source, "hf://") {
		return RepoLocation{
			RepoID:   source,
			RepoType: ModelRepo,
			Revision: defaultRevision,
		}, nil
	}

	body := strings.TrimPrefix(source, "hf://")
	repoType := ModelRepo

	prefix, rest, hasPrefix := strings.Cut(body, "/")

	if hasPrefix {
		if parsed, err := parseRepoType(prefix); err == nil && RepoType(prefix) != "" {
			repoType = parsed
			body = rest
		}
	}

	revision := defaultRevision

	if before, after, ok := strings.Cut(body, "@"); ok {
		body = before
		revision = normalizeRevision(after)
	}

	if strings.TrimSpace(body) == "" {
		return RepoLocation{}, fmt.Errorf("hub: repo id is required")
	}

	return RepoLocation{
		RepoID:   body,
		RepoType: repoType,
		Revision: revision,
	}, nil
}

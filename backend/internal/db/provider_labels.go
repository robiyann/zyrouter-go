package db

import "strings"

// ProviderNodeName exposes only the friendly provider-node name needed by the
// shared label formatter. It keeps labels independent from the db package.
func (r *Repo) ProviderNodeName(provider string) (string, error) {
	node, _, err := r.GetProviderNodeByID(provider)
	if err != nil || node == nil || node.Name == nil {
		return "", err
	}
	return strings.TrimSpace(*node.Name), nil
}

// ProviderNodePrefix exposes the provider-node client prefix needed by the
// shared label formatter.
func (r *Repo) ProviderNodePrefix(provider string) (string, error) {
	_, nodeData, err := r.GetProviderNodeByID(provider)
	if err != nil || nodeData == nil {
		return "", err
	}
	return strings.TrimSpace(nodeData.Prefix), nil
}

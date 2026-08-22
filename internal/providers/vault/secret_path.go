package vault

import (
	"fmt"
	"strings"
)

type SecretPath struct {
	Path string
	Key  string
}

func parseSecretPath(path string) (*SecretPath, error) {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 || idx == len(path)-1 {
		return nil, fmt.Errorf("secret path %q does not contain a key segment", path)
	}

	return &SecretPath{
		Path: path[:idx],
		Key:  path[idx+1:],
	}, nil
}

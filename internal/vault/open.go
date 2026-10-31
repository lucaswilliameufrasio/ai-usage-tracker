package vault

import (
	"path/filepath"
	"strings"
)

const (
	KindOwn       = "own"
	KindCiphera   = "ciphera"
	KindHashicorp = "hashicorp"
	KindAWS       = "aws"
	KindInfisical = "infisical"
)

// OpenConfigured builds the backends whose opening material is present.
// It does not call the remote service. A missing backend is absent from the map.
func OpenConfigured(getenv func(string) string, dataDir string) map[string]Box {
	out := map[string]Box{}
	rawKey := strings.TrimSpace(getenv("TRACKER_VAULT_KEY"))
	if rawKey != "" {
		key, err := DecodeKey(rawKey)
		if err == nil {
			opened, openErr := OpenOwn(filepath.Join(dataDir, "vault"), key)
			for i := range key {
				key[i] = 0
			}
			if openErr == nil {
				out[KindOwn] = opened
			}
		}
	}
	if ciphera, ok := openCiphera(getenv); ok {
		out[KindCiphera] = ciphera
	}
	if hashicorp, ok := openHashicorp(getenv); ok {
		out[KindHashicorp] = hashicorp
	}
	if aws, ok := openAWS(getenv); ok {
		out[KindAWS] = aws
	}
	if infisical, ok := openInfisical(getenv); ok {
		out[KindInfisical] = infisical
	}
	return out
}

func openCiphera(getenv func(string) string) (*Ciphera, bool) {
	origin := strings.TrimSpace(getenv("TRACKER_CIPHERA_ORIGIN"))
	project := strings.TrimSpace(getenv("TRACKER_CIPHERA_PROJECT"))
	environment := strings.TrimSpace(getenv("TRACKER_CIPHERA_ENVIRONMENT"))
	token := strings.TrimSpace(getenv("TRACKER_CIPHERA_TOKEN"))
	serviceKey := strings.TrimSpace(getenv("TRACKER_CIPHERA_SERVICE_KEY"))
	if origin == "" || project == "" || environment == "" {
		return nil, false
	}
	if token == "" && serviceKey == "" {
		return nil, false
	}
	return &Ciphera{
		Origin:      origin,
		Project:     project,
		Environment: environment,
		Token:       token,
		ServiceKey:  serviceKey,
	}, true
}

func openHashicorp(getenv func(string) string) (*HashiCorp, bool) {
	address := strings.TrimSpace(getenv("TRACKER_HASHICORP_ADDR"))
	token := strings.TrimSpace(getenv("TRACKER_HASHICORP_TOKEN"))
	if address == "" || token == "" {
		return nil, false
	}
	return &HashiCorp{
		Address:   address,
		Token:     token,
		Mount:     strings.TrimSpace(getenv("TRACKER_HASHICORP_MOUNT")),
		Prefix:    strings.TrimSpace(getenv("TRACKER_HASHICORP_PREFIX")),
		Namespace: strings.TrimSpace(getenv("TRACKER_HASHICORP_NAMESPACE")),
	}, true
}

func openAWS(getenv func(string) string) (*SecretsManager, bool) {
	region := strings.TrimSpace(getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(getenv("AWS_DEFAULT_REGION"))
	}
	access := strings.TrimSpace(getenv("AWS_ACCESS_KEY_ID"))
	secret := strings.TrimSpace(getenv("AWS_SECRET_ACCESS_KEY"))
	if region == "" || access == "" || secret == "" {
		return nil, false
	}
	return &SecretsManager{
		Region:          region,
		AccessKeyID:     access,
		SecretAccessKey: secret,
		SessionToken:    strings.TrimSpace(getenv("AWS_SESSION_TOKEN")),
		Prefix:          strings.TrimSpace(getenv("TRACKER_AWS_SECRET_PREFIX")),
	}, true
}

func openInfisical(getenv func(string) string) (*Infisical, bool) {
	origin := strings.TrimSpace(getenv("TRACKER_INFISICAL_ORIGIN"))
	project := strings.TrimSpace(getenv("TRACKER_INFISICAL_PROJECT"))
	environment := strings.TrimSpace(getenv("TRACKER_INFISICAL_ENVIRONMENT"))
	clientID := strings.TrimSpace(getenv("TRACKER_INFISICAL_CLIENT_ID"))
	clientSecret := strings.TrimSpace(getenv("TRACKER_INFISICAL_CLIENT_SECRET"))
	if origin == "" || project == "" || environment == "" || clientID == "" || clientSecret == "" {
		return nil, false
	}
	return &Infisical{
		Origin:       origin,
		ProjectID:    project,
		Environment:  environment,
		SecretPath:   strings.TrimSpace(getenv("TRACKER_INFISICAL_PATH")),
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, true
}

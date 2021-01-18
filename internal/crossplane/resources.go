package crossplane

import (
	"context"
	"strconv"

	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

// CredentialExtractor retrieve binding credentials.
type CredentialExtractor interface {
	GetCredentials(ctx context.Context) (interface{}, error)
}

// SecretCredentials encapsulates a password retrieved from a k8s secret.
type SecretCredentials struct {
	Endpoint string
	Port     int32
	Password string
}

// SecretResource is a credential extractor.
type SecretResource struct {
	namespace   string
	resourceRef corev1.ObjectReference
	cp          *Crossplane
}

// NewSecretResource instantiates a SecretResource to be used as a CredentialExtractor.
func NewSecretResource(namespace string, resourceRef corev1.ObjectReference, cp *Crossplane) *SecretResource {
	return &SecretResource{
		namespace:   namespace,
		resourceRef: resourceRef,
		cp:          cp,
	}
}

// GetCredentials retrieves the secret specified by the resourceRef and returns the password within that secret.
func (sr *SecretResource) GetCredentials(ctx context.Context) (interface{}, error) {
	s, err := sr.cp.getCredentials(ctx, sr.resourceRef.Name)
	if err != nil {
		return nil, err
	}

	sport := string(s.Data[xrv1.ResourceCredentialsSecretPortKey])
	port, err := strconv.Atoi(sport)
	if err != nil {
		return nil, err
	}

	creds := SecretCredentials{
		Endpoint: string(s.Data[xrv1.ResourceCredentialsSecretEndpointKey]),
		Port:     int32(port),
		Password: string(s.Data[xrv1.ResourceCredentialsSecretPasswordKey]),
	}
	return &creds, nil
}

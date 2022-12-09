// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package ocm

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Masterminds/semver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/attrs/signingattr"
	ocmreg "github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/signing"

	csdk "github.com/open-component-model/ocm-controllers-sdk"

	"github.com/open-component-model/ocm-controller/api/v1alpha1"
)

// Verifier takes a Component and runs OCM verification on it.
type Verifier interface {
	VerifyComponent(ctx context.Context, obj *v1alpha1.ComponentVersion, version string) (bool, error)
}

// Fetcher gets information about an OCM component Version based on a k8s component Version.
type Fetcher interface {
	GetComponentVersion(ctx context.Context, obj *v1alpha1.ComponentVersion, name, version string) (ocm.ComponentVersionAccess, error)
	GetLatestComponentVersion(ctx context.Context, obj *v1alpha1.ComponentVersion) (string, error)
	ListComponentVersions(ctx ocm.Context, obj *v1alpha1.ComponentVersion) ([]Version, error)
}

// FetchVerifier can fetch and verify components.
type FetchVerifier interface {
	Verifier
	Fetcher
}

// Client implements the OCM fetcher interface.
type Client struct {
	client client.Client
}

var _ FetchVerifier = &Client{}

// NewClient creates a new fetcher Client using the provided k8s client.
func NewClient(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

// GetComponentVersion returns a component version. It's the caller's responsibility to clean it up and close the component version once done with it.
func (c *Client) GetComponentVersion(ctx context.Context, obj *v1alpha1.ComponentVersion, name, version string) (ocm.ComponentVersionAccess, error) {
	log := log.FromContext(ctx)

	octx := ocm.ForContext(ctx)
	// configure registry credentials
	if err := csdk.ConfigureCredentials(ctx, octx, c.client, obj.Spec.Repository.URL, obj.Spec.Repository.SecretRef.Name, obj.Namespace); err != nil {
		log.V(4).Error(err, "failed to find credentials")
		// ignore not found errors for now
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to configure credentials for component: %w", err)
		}
	}
	repo, err := octx.RepositoryForSpec(ocmreg.NewRepositorySpec(obj.Spec.Repository.URL, nil))
	if err != nil {
		return nil, fmt.Errorf("failed to get repository for spec: %w", err)
	}
	defer repo.Close()

	cv, err := repo.LookupComponentVersion(name, version)
	if err != nil {
		return nil, fmt.Errorf("failed to look up component version: %w", err)
	}

	return cv, nil
}

func (c *Client) VerifyComponent(ctx context.Context, obj *v1alpha1.ComponentVersion, version string) (bool, error) {
	log := log.FromContext(ctx)

	octx := ocm.ForContext(ctx)
	// configure registry credentials
	if err := csdk.ConfigureCredentials(ctx, octx, c.client, obj.Spec.Repository.URL, obj.Spec.Repository.SecretRef.Name, obj.Namespace); err != nil {
		log.V(4).Error(err, "failed to find credentials")
		// ignore not found errors for now
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to configure credentials for component: %w", err)
		}
	}

	repo, err := octx.RepositoryForSpec(ocmreg.NewRepositorySpec(obj.Spec.Repository.URL, nil))
	if err != nil {
		return false, fmt.Errorf("failed to get repository for spec: %w", err)
	}
	defer repo.Close()

	cv, err := repo.LookupComponentVersion(obj.Spec.Component, version)
	if err != nil {
		return false, fmt.Errorf("failed to look up component version: %w", err)
	}
	defer cv.Close()

	resolver := ocm.NewCompoundResolver(repo)

	for _, signature := range obj.Spec.Verify {
		cert, err := c.getPublicKey(ctx, obj.Namespace, signature.PublicKey.SecretRef.Name, signature.Name)
		if err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		opts := signing.NewOptions(
			signing.VerifySignature(signature.Name),
			signing.Resolver(resolver),
			signing.VerifyDigests(),
			signing.PublicKey(signature.Name, cert),
		)

		if err := opts.Complete(signingattr.Get(octx)); err != nil {
			return false, fmt.Errorf("verify error: %w", err)
		}

		dig, err := signing.Apply(nil, nil, cv, opts)
		if err != nil {
			return false, err
		}

		var value string
		for _, os := range cv.GetDescriptor().Signatures {
			if os.Name == signature.Name {
				value = os.Digest.Value
				break
			}
		}
		if value == "" {
			return false, fmt.Errorf("signature with name '%s' not found in the list of provided ocm signatures", signature.Name)
		}
		if dig.Value != value {
			return false, fmt.Errorf("%s signature did not match key value", signature.Name)
		}
	}

	return true, nil
}

func (c *Client) getPublicKey(ctx context.Context, namespace, name, signature string) ([]byte, error) {
	var secret corev1.Secret
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.client.Get(ctx, secretKey, &secret); err != nil {
		return nil, err
	}

	for key, value := range secret.Data {
		if key == signature {
			return value, nil
		}
	}

	return nil, errors.New("public key not found")
}

func (c *Client) GetLatestComponentVersion(ctx context.Context, obj *v1alpha1.ComponentVersion) (string, error) {
	ocmCtx := ocm.ForContext(ctx)

	versions, err := c.ListComponentVersions(ocmCtx, obj)
	if err != nil {
		return "", fmt.Errorf("failed to get component versions: %w", err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for component '%s'", obj.Spec.Component)
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].semver.GreaterThan(versions[j].semver)
	})

	return versions[0].version, nil
}

// Version has two values to be able to sort a list but still return the actual Version.
// The Version might contain a `v`.
type Version struct {
	semver  *semver.Version
	version string
}

func (c *Client) ListComponentVersions(octx ocm.Context, obj *v1alpha1.ComponentVersion) ([]Version, error) {
	repo, err := octx.RepositoryForSpec(ocmreg.NewRepositorySpec(obj.Spec.Repository.URL, nil))
	if err != nil {
		return nil, fmt.Errorf("failed to get repository for spec: %w", err)
	}
	defer repo.Close()

	// get the component Version
	cv, err := repo.LookupComponent(obj.Spec.Component)
	if err != nil {
		return nil, fmt.Errorf("component error: %w", err)
	}
	defer cv.Close()

	versions, err := cv.ListVersions()
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for component: %w", err)
	}

	var result []Version
	for _, v := range versions {
		parsed, err := semver.NewVersion(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Version '%s': %w", v, err)
		}
		result = append(result, Version{
			semver:  parsed,
			version: v,
		})
	}
	return result, nil
}
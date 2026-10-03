package cloudru

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru/certificateapi"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

func (p *Provider) addCertificates(ctx context.Context, secrets map[string]contracts.Secret) error {
	certificates, err := p.certificateManager.ListCertificates(ctx, p.cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("list certificates: %w", err)
	}
	payloads := make(map[string][]byte)
	for _, certificate := range certificates {
		mapped, values, mapErr := p.loadCertificate(ctx, certificate)
		if mapErr != nil {
			return fmt.Errorf("load certificate %q: %w", certificate.ID, mapErr)
		}
		for path, secret := range mapped {
			// Reject both scoped Secret Manager collisions and multiple certificates with the same domain.
			for _, existing := range secrets {
				if existing.Path == path {
					return fmt.Errorf("certificate secret name collision at %q", path)
				}
			}
			secrets[path] = secret
			payloads[path] = values[path]
		}
	}
	// Publish only complete, validated pairs from a successful listing.
	p.certificateMu.Lock()
	p.certificatePayloads = payloads
	p.certificateMu.Unlock()
	return nil
}

func (p *Provider) loadCertificate(
	ctx context.Context, certificate certificateapi.Certificate,
) (map[string]contracts.Secret, map[string][]byte, error) {
	if certificate.ID == "" {
		return nil, nil, errors.New("missing certificate ID")
	}
	versions, err := p.certificateManager.ListVersions(ctx, certificate.ID)
	if err != nil {
		return nil, nil, err
	}
	versionID, err := activeCertificateVersion(versions, time.Now())
	if err != nil || versionID == "" {
		return nil, nil, err
	}
	material, err := p.certificateManager.GetMaterial(ctx, certificate.ID, versionID)
	if err != nil {
		return nil, nil, err
	}
	if material.Version.ID.String() != versionID ||
		(material.Version.CertificateID != "" && material.Version.CertificateID != certificate.ID) {
		return nil, nil, errors.New("certificate material belongs to a different version")
	}
	if !usableCertificateVersion(material.Version) {
		return nil, nil, errors.New("selected certificate version is no longer usable")
	}
	return mapCertificate(certificate, material, time.Now())
}

func usableCertificateVersion(version certificateapi.Version) bool {
	return version.Enabled && (version.Status == "VERSION_STATUS_READY" || version.Status == "VERSION_STATUS_WARNING")
}

func activeCertificateVersion(versions []certificateapi.Version, now time.Time) (string, error) {
	var latest uint64
	for _, version := range versions {
		if !usableCertificateVersion(version) {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, version.ExpiresAt)
		if err != nil {
			return "", fmt.Errorf("parse certificate version expiration: %w", err)
		}
		if !expiresAt.After(now) {
			continue
		}
		id, err := strconv.ParseUint(version.ID.String(), 10, 64)
		if err != nil || id == 0 {
			return "", errors.New("invalid certificate version ID")
		}
		if id > latest {
			latest = id
		}
	}
	if latest == 0 {
		return "", nil
	}
	return strconv.FormatUint(latest, 10), nil
}

func mapCertificate(
	certificate certificateapi.Certificate, material certificateapi.Material, now time.Time,
) (map[string]contracts.Secret, map[string][]byte, error) {
	fullchain := strings.TrimSpace(material.Version.ServerCertificate) + "\n"
	if chain := strings.TrimSpace(material.Version.IntermediateCertificate); chain != "" {
		fullchain += chain + "\n"
	}
	leaf, err := validateCertificateMaterial(fullchain, material.PrivateKey)
	if err != nil {
		return nil, nil, err
	}
	if !leaf.NotAfter.After(now) || leaf.NotBefore.After(now) {
		return nil, nil, nil
	}
	domains := certificateDomains(leaf)
	if len(domains) == 0 {
		return nil, nil, errors.New("certificate has no DNS names or common name")
	}
	baseDomains := make([]string, 0, len(domains))
	for _, domain := range domains {
		baseDomains = append(baseDomains, strings.TrimPrefix(domain, "*."))
	}
	slices.Sort(baseDomains)
	base := "certs-" + baseDomains[0]
	if !validCertificateSecretName(base + "-crt") {
		return nil, nil, errors.New("certificate domain cannot be represented as a Docker secret name")
	}
	secrets := make(map[string]contracts.Secret)
	payloads := make(map[string][]byte)
	for _, part := range []struct{ suffix, label, value string }{
		{"crt", "certificate", fullchain},
		{"pk", "private_key", material.PrivateKey},
	} {
		path := base + "-" + part.suffix
		secrets[path] = contracts.Secret{
			Path:        path,
			FullPath:    "certificate-manager/" + certificate.ID + "/" + part.suffix,
			VersionID:   certificate.ID + "-" + material.Version.ID.String(),
			Description: certificate.Description,
			Labels: map[string]string{
				"type":                   "certificate",
				"certificate.domains":    strings.Join(domains, ","),
				"certificate.expires_at": leaf.NotAfter.UTC().Format(time.RFC3339),
				"certificate.part":       part.label,
			},
		}
		payloads[path] = []byte(part.value)
	}
	return secrets, payloads, nil
}

func validateCertificateMaterial(fullchain, privateKey string) (*x509.Certificate, error) {
	remaining := []byte(fullchain)
	var leaf *x509.Certificate
	for len(strings.TrimSpace(string(remaining))) > 0 {
		if !strings.HasPrefix(strings.TrimSpace(string(remaining)), "-----BEGIN CERTIFICATE-----") {
			return nil, errors.New("invalid certificate PEM chain")
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, errors.New("invalid certificate PEM chain")
		}
		parsed, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse X.509 certificate: %w", err)
		}
		if leaf == nil {
			leaf = parsed
		}
		remaining = rest
	}
	if leaf == nil {
		return nil, errors.New("missing leaf certificate")
	}
	if _, err := tls.X509KeyPair([]byte(fullchain), []byte(privateKey)); err != nil {
		// Parsing errors are safe; payload/key contents are never included.
		return nil, fmt.Errorf("validate certificate/private key pair: %w", err)
	}
	return leaf, nil
}

func certificateDomains(leaf *x509.Certificate) []string {
	domains := slices.Clone(leaf.DNSNames)
	if len(domains) == 0 && leaf.Subject.CommonName != "" {
		domains = []string{leaf.Subject.CommonName}
	}
	for i, domain := range domains {
		domains[i] = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	}
	slices.Sort(domains)
	domains = slices.Compact(domains)
	return slices.DeleteFunc(domains, func(domain string) bool { return domain == "" })
}

func validCertificateSecretName(name string) bool {
	if len(name) > secretname.MaxLength {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '.' && char != '_' {
			return false
		}
	}
	return true
}

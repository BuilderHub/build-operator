package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	buildkitv1alpha1 "github.com/builderhub/build-operator/api/v1alpha1"
)

const (
	// caSecretName is the per-namespace (per-org) CA used to sign builder server
	// certificates and client certificates. It is created on demand and not owned
	// by any single builder so it survives individual builder deletions.
	caSecretName = "builderhub-builder-ca"

	caValidity     = 10 * 365 * 24 * time.Hour
	serverValidity = 365 * 24 * time.Hour
	// renewBefore triggers regeneration of the server cert when it is close to expiry.
	renewBefore = 30 * 24 * time.Hour

	certHostAnnotation = "builder-hub.dev/cert-host"

	tlsMountPath = "/etc/buildkit-tls"
)

func builderTLSSecretName(b *buildkitv1alpha1.BuildkitBuilder) string {
	return fmt.Sprintf("builder-%s-tls", b.Name)
}

// exposureTLSSecretName returns the server-cert Secret name when exposed, else "".
func exposureTLSSecretName(b *buildkitv1alpha1.BuildkitBuilder) string {
	if !exposureEnabled(b) {
		return ""
	}
	return builderTLSSecretName(b)
}

// ensureNamespaceCA returns the namespace CA, creating it if it does not exist.
func (r *BuildkitBuilderReconciler) ensureNamespaceCA(ctx context.Context, namespace string) (caCertPEM []byte, caCert *x509.Certificate, caKey *rsa.PrivateKey, err error) {
	sec := &corev1.Secret{}
	getErr := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: caSecretName}, sec)
	if getErr == nil {
		caCert, caKey, err = parseCertAndKey(sec.Data["ca.crt"], sec.Data["ca.key"])
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parsing existing CA: %w", err)
		}
		return sec.Data["ca.crt"], caCert, caKey, nil
	}
	if !errors.IsNotFound(getErr) {
		return nil, nil, nil, getErr
	}

	caCert, caKey, caCertPEM, caKeyPEM, err := generateCA(namespace)
	if err != nil {
		return nil, nil, nil, err
	}
	sec = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: caSecretName},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt": caCertPEM,
			"ca.key": caKeyPEM,
		},
	}
	if err := r.Create(ctx, sec); err != nil {
		if errors.IsAlreadyExists(err) {
			// Lost a race; re-read.
			if getErr := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: caSecretName}, sec); getErr != nil {
				return nil, nil, nil, getErr
			}
			caCert, caKey, err = parseCertAndKey(sec.Data["ca.crt"], sec.Data["ca.key"])
			if err != nil {
				return nil, nil, nil, err
			}
			return sec.Data["ca.crt"], caCert, caKey, nil
		}
		return nil, nil, nil, err
	}
	return caCertPEM, caCert, caKey, nil
}

// ensureBuilderServerCert ensures the per-builder mTLS server certificate Secret
// (ca.crt, tls.crt, tls.key) exists and matches the builder host. Returns the
// Secret name to mount into buildkitd.
func (r *BuildkitBuilderReconciler) ensureBuilderServerCert(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) (string, error) {
	host := b.Spec.Exposure.Host
	if host == "" {
		return "", fmt.Errorf("exposure.host is required to issue a server certificate")
	}

	caCertPEM, caCert, caKey, err := r.ensureNamespaceCA(ctx, b.Namespace)
	if err != nil {
		return "", err
	}

	name := builderTLSSecretName(b)
	existing := &corev1.Secret{}
	getErr := r.Get(ctx, client.ObjectKey{Namespace: b.Namespace, Name: name}, existing)
	if getErr == nil && !serverCertNeedsRenewal(existing, host) {
		return name, nil
	}
	if getErr != nil && !errors.IsNotFound(getErr) {
		return "", getErr
	}

	certPEM, keyPEM, err := signServerCert(caCert, caKey, host)
	if err != nil {
		return "", err
	}

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   b.Namespace,
			Name:        name,
			Annotations: map[string]string{certHostAnnotation: host},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt":  caCertPEM,
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
	if err := controllerutil.SetControllerReference(b, desired, r.Scheme); err != nil {
		return "", err
	}

	if errors.IsNotFound(getErr) {
		if err := r.Create(ctx, desired); err != nil && !errors.IsAlreadyExists(err) {
			return "", err
		}
		return name, nil
	}
	existing.Data = desired.Data
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	existing.Annotations[certHostAnnotation] = host
	if err := r.Update(ctx, existing); err != nil {
		return "", err
	}
	return name, nil
}

// deleteBuilderServerCert removes the per-builder TLS secret (best effort).
func (r *BuildkitBuilderReconciler) deleteBuilderServerCert(ctx context.Context, b *buildkitv1alpha1.BuildkitBuilder) error {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.Namespace, Name: builderTLSSecretName(b)}}
	if err := r.Delete(ctx, sec); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func serverCertNeedsRenewal(sec *corev1.Secret, host string) bool {
	if sec.Annotations[certHostAnnotation] != host {
		return true
	}
	cert, _, err := parseCertAndKey(sec.Data["tls.crt"], sec.Data["tls.key"])
	if err != nil {
		return true
	}
	return time.Now().Add(renewBefore).After(cert.NotAfter)
}

func generateCA(namespace string) (*x509.Certificate, *rsa.PrivateKey, []byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	serial, err := randSerial()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: fmt.Sprintf("builderhub-builder-ca:%s", namespace)},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return cert, key, encodeCertPEM(der), encodeKeyPEM(key), nil
}

func signServerCert(caCert *x509.Certificate, caKey *rsa.PrivateKey, host string) (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(serverValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return encodeCertPEM(der), encodeKeyPEM(key), nil
}

func parseCertAndKey(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	cb, _ := pem.Decode(certPEM)
	if cb == nil {
		return nil, nil, fmt.Errorf("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, nil, fmt.Errorf("invalid key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func randSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func encodeCertPEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func encodeKeyPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

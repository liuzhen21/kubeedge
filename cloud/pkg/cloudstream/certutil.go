package cloudstream

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	pkgutil "github.com/kubeedge/kubeedge/pkg/util"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
)

// Copy from edgewize

func SignCloudCoreCert(cacrt, cakey []byte) ([]byte, []byte, error) {
	podIP, err := pkgutil.GetLocalIP(pkgutil.GetHostname())
	if err != nil {
		klog.Errorf("Failed to get Local IP address: %v", err)
		return nil, nil, err
	}
	klog.Infoln("pod ip", podIP)

	cfg := &certutil.Config{
		CommonName:   "EdgeWize",
		Organization: []string{"EdgeWize"},
		Usages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		AltNames: certutil.AltNames{
			IPs: []net.IP{net.ParseIP(podIP)}, // TODO
		},
	}
	var keyDER []byte
	caCert, err := x509.ParseCertificate(cacrt)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse a caCert from the given ASN.1 DER data, err: %v", err)
	}
	serverKey, err := NewPrivateKey(caCert.SignatureAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate a privateKey, err: %v", err)
	}
	var caKey crypto.Signer
	switch caCert.SignatureAlgorithm {
	case x509.ECDSAWithSHA256:
		caKey, err = x509.ParseECPrivateKey(cakey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse ECPrivateKey, err: %v", err)
		}
		keyDER, err = x509.MarshalECPrivateKey(serverKey.(*ecdsa.PrivateKey))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert an EC private key to SEC 1, ASN.1 DER form, err: %v", err)
		}
	case x509.SHA256WithRSA:
		caKey, err = x509.ParsePKCS1PrivateKey(cakey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse PKCS1PrivateKey, err: %v", err)
		}
		keyDER = x509.MarshalPKCS1PrivateKey(serverKey.(*rsa.PrivateKey))
	default:
		return nil, nil, fmt.Errorf("unsupport signature algorithm: %s", caCert.SignatureAlgorithm.String())
	}

	certDER, err := NewCertFromCa(cfg, caCert, serverKey.Public(), caKey, 365*100)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate a certificate using the given CA certificate and key, err: %v", err)
	}
	return certDER, keyDER, nil
}

// NewPrivateKey creates a private key
func NewPrivateKey(algorithm x509.SignatureAlgorithm) (crypto.Signer, error) {
	switch algorithm {
	case x509.ECDSAWithSHA256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case x509.SHA256WithRSA:
		return rsa.GenerateKey(rand.Reader, 2048)
	default:
		return nil, fmt.Errorf("unsepport signature algorithm: %s", algorithm.String())
	}
}

// NewCertFromCa creates a signed certificate using the given CA certificate and key
func NewCertFromCa(cfg *certutil.Config, caCert *x509.Certificate, serverKey crypto.PublicKey, caKey crypto.Signer, validalityPeriod time.Duration) ([]byte, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    time.Now().UTC(),
		NotAfter:     time.Now().Add(time.Hour * 24 * validalityPeriod),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, &certTmpl, caCert, serverKey, caKey)
	if err != nil {
		return nil, err
	}
	return certDERBytes, nil
}

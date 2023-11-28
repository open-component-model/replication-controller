package sign

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
)

const bitSize = 4096

// GenerateSigningKeyPEMPair creates a public and private keypair for signing OCM components.
func GenerateSigningKeyPEMPair() ([]byte, []byte, error) {
	privateKey, err := generatePrivateKey(bitSize)
	if err != nil {
		return nil, nil, err
	}

	privatePem := encodePrivateKeyToPEM(privateKey)
	publicPem := encodePublicKeyToPEM(&privateKey.PublicKey)

	return privatePem, publicPem, nil
}

// generatePrivateKey creates an RSA Private Key of specified byte size.
func generatePrivateKey(bitSize int) (*rsa.PrivateKey, error) {
	// Private Key generation
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return nil, err
	}

	// Validate Private Key
	err = privateKey.Validate()
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// encodePrivateKeyToPEM encodes Private Key from RSA to PEM format.
func encodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	// Get ASN.1 DER format
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privatePEM := pem.EncodeToMemory(&privBlock)

	return privatePEM
}

// encodePrivateKeyToPEM encodes Private Key from RSA to PEM format.
func encodePublicKeyToPEM(publicKey *rsa.PublicKey) []byte {
	// Get ASN.1 DER format
	key := x509.MarshalPKCS1PublicKey(publicKey)

	// pem.Block
	block := pem.Block{
		Type:    "RSA PUBLIC KEY",
		Headers: nil,
		Bytes:   key,
	}

	// Private key in PEM format
	keyPem := pem.EncodeToMemory(&block)

	return keyPem
}

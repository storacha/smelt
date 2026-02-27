package identity

import (
	crypto_ed25519 "crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
)

// Identity holds the service's cryptographic identity.
type Identity struct {
	Signer principal.Signer
}

// New creates a new identity. If privateKeyBase64 is empty, generates a new key.
func New(privateKeyBase64 string) (*Identity, error) {
	var signer principal.Signer
	var err error

	if privateKeyBase64 == "" {
		// Generate ephemeral identity
		signer, err = ed25519.Generate()
		if err != nil {
			return nil, fmt.Errorf("failed to generate signer: %w", err)
		}
	} else {
		// Decode provided key
		keyBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode private key: %w", err)
		}
		signer, err = ed25519.Decode(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to create signer from key: %w", err)
		}
	}

	return &Identity{
		Signer: signer,
	}, nil
}

// DID returns the service's DID.
func (i *Identity) DID() string {
	return i.Signer.DID().String()
}

// UnderlyingKeyDID returns the underlying did:key for wrapped signers.
// For unwrapped signers, returns the same as DID().
func (i *Identity) UnderlyingKeyDID() string {
	// Try to unwrap if it's a wrapped signer
	if wrapped, ok := i.Signer.(signer.WrappedSigner); ok {
		return wrapped.Unwrap().DID().String()
	}
	return i.Signer.DID().String()
}

// DIDDocument returns a DID document for did:web resolution.
// This enables other services to verify signatures from this service.
func (i *Identity) DIDDocument() map[string]interface{} {
	serviceDID := i.DID()
	keyDID := i.UnderlyingKeyDID()

	// Extract the multibase public key from the did:key
	// did:key format is "did:key:z6Mk..." where z6Mk... is the multibase-encoded public key
	publicKeyMultibase := strings.TrimPrefix(keyDID, "did:key:")

	keyID := serviceDID + "#key-0"

	return map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		"id": serviceDID,
		"verificationMethod": []map[string]interface{}{
			{
				"id":                 keyID,
				"type":               "Ed25519VerificationKey2020",
				"controller":         serviceDID,
				"publicKeyMultibase": publicKeyMultibase,
			},
		},
		"authentication":  []string{keyID},
		"assertionMethod": []string{keyID},
	}
}

// NewFromPEMFile creates a new identity from an Ed25519 PEM key file.
func NewFromPEMFile(keyFilePath string) (*Identity, error) {
	keySigner, err := signerFromEd25519PEMFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key from PEM file: %w", err)
	}
	return &Identity{Signer: keySigner}, nil
}

// NewFromPEMFileWithDID creates a new identity from an Ed25519 PEM key file,
// optionally wrapping it with a did:web identity.
// When serviceDID is provided (e.g., "did:web:upload"), the underlying did:key
// signer is wrapped so the service presents itself as the did:web identity
// and accepts UCANs addressed to that did:web.
func NewFromPEMFileWithDID(keyFilePath string, serviceDID string) (*Identity, error) {
	keySigner, err := signerFromEd25519PEMFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key from PEM file: %w", err)
	}

	// If serviceDID is provided, wrap the signer with the did:web identity
	if serviceDID != "" {
		d, err := did.Parse(serviceDID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service DID %q: %w", serviceDID, err)
		}

		wrappedSigner, err := signer.Wrap(keySigner, d)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap signer with DID %q: %w", serviceDID, err)
		}

		return &Identity{Signer: wrappedSigner}, nil
	}

	return &Identity{Signer: keySigner}, nil
}

// signerFromEd25519PEMFile loads an Ed25519 private key from a PKCS#8 PEM file.
func signerFromEd25519PEMFile(path string) (principal.Signer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}
	defer f.Close()

	pemData, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var privateKey *crypto_ed25519.PrivateKey
	rest := pemData
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining

		if block.Type == "PRIVATE KEY" {
			parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
			}

			key, ok := parsedKey.(crypto_ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("key is not an Ed25519 private key")
			}
			privateKey = &key
			break
		}
	}

	if privateKey == nil {
		return nil, fmt.Errorf("no PRIVATE KEY block found in PEM file")
	}

	return ed25519.FromRaw(*privateKey)
}

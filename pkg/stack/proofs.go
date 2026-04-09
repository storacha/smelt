package stack

import (
	crypto_ed25519 "crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/go-ucanto/ucan"
)

// proofSpec defines a UCAN delegation proof to generate.
type proofSpec struct {
	issuerKeyName string   // Key file name without .pem extension
	issuerDidWeb  string   // did:web identifier for the issuer
	audienceDid   string   // DID of the audience
	capabilities  []string // Capabilities to delegate (e.g., "claim/cache")
	outputFile    string   // Output file name in proofs directory
}

// proofSpecs defines all the delegation proofs needed for the stack.
// These correspond to what generate-proofs.sh creates.
var proofSpecs = []proofSpec{
	{
		issuerKeyName: "indexer",
		issuerDidWeb:  "did:web:indexer",
		audienceDid:   "did:web:delegator",
		capabilities:  []string{"claim/cache"},
		outputFile:    "indexing-service-proof.txt",
	},
	{
		issuerKeyName: "etracker",
		issuerDidWeb:  "did:web:etracker",
		audienceDid:   "did:web:delegator",
		capabilities:  []string{"egress/track"},
		outputFile:    "egress-tracking-proof.txt",
	},
	{
		issuerKeyName: "piri",
		audienceDid:   "did:web:upload",
		capabilities:  []string{"blob/allocate", "blob/accept", "blob/replica/allocate", "pdp/info"},
		outputFile:    "piri-proof.txt",
	},
}

// generateProofs generates all UCAN delegation proofs needed for service communication.
func generateProofs(tempDir string) error {
	keysDir := filepath.Join(tempDir, "generated", "keys")
	proofsDir := filepath.Join(tempDir, "generated", "proofs")

	for _, spec := range proofSpecs {
		if err := generateDelegation(keysDir, proofsDir, spec); err != nil {
			return fmt.Errorf("generate %s: %w", spec.outputFile, err)
		}
	}

	return nil
}

// generateDelegation creates a single UCAN delegation and writes it to a file.
func generateDelegation(keysDir, proofsDir string, spec proofSpec) error {
	// Load issuer key from PEM file
	issuerKey, err := loadSignerFromPEM(filepath.Join(keysDir, spec.issuerKeyName+".pem"))
	if err != nil {
		return fmt.Errorf("load issuer key: %w", err)
	}

	var issuer ucan.Signer
	if spec.issuerDidWeb != "" {
		// Parse the did:web for wrapping
		issuerDidWeb, err := did.Parse(spec.issuerDidWeb)
		if err != nil {
			return fmt.Errorf("parse issuer did:web: %w", err)
		}

		// Wrap with did:web identity
		issuer, err = signer.Wrap(issuerKey, issuerDidWeb)
		if err != nil {
			return fmt.Errorf("wrap issuer: %w", err)
		}
	} else {
		issuer = issuerKey
	}

	// Parse audience DID
	audience, err := did.Parse(spec.audienceDid)
	if err != nil {
		return fmt.Errorf("parse audience: %w", err)
	}

	// Create capabilities with the issuer's DID as the resource
	caps := make([]ucan.Capability[ucan.NoCaveats], len(spec.capabilities))
	for i, cap := range spec.capabilities {
		caps[i] = ucan.NewCapability(cap, issuer.DID().String(), ucan.NoCaveats{})
	}

	// Create delegation with no expiration
	dlg, err := delegation.Delegate(issuer, audience, caps, delegation.WithNoExpiration())
	if err != nil {
		return fmt.Errorf("create delegation: %w", err)
	}

	// Format as base64url-encoded CAR
	formatted, err := delegation.Format(dlg)
	if err != nil {
		return fmt.Errorf("format delegation: %w", err)
	}

	// Write to file
	outputPath := filepath.Join(proofsDir, spec.outputFile)
	if err := os.WriteFile(outputPath, []byte(formatted), 0644); err != nil {
		return fmt.Errorf("write proof file: %w", err)
	}

	return nil
}

// loadSignerFromPEM loads an Ed25519 private key from a PEM file.
// This follows the same pattern as go-mkdelegation's parsePrivateKeyPEM.
func loadSignerFromPEM(path string) (principal.Signer, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read PEM file: %w", err)
	}

	// Find the PRIVATE KEY block
	rest := pemData
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining

		if block.Type == "PRIVATE KEY" {
			// Parse PKCS#8 format
			parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
			}

			// Cast to Ed25519 private key
			key, ok := parsedKey.(crypto_ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("key is not an Ed25519 private key")
			}

			// Convert to go-ucanto signer
			return ed25519.FromRaw(key)
		}
	}

	return nil, fmt.Errorf("no PRIVATE KEY block found in PEM file")
}

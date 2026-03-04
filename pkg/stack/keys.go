package stack

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// serviceKeys is the list of services that need Ed25519 keys.
var serviceKeys = []string{
	"piri",
	"upload",
	"indexer",
	"delegator",
	"signing-service",
	"etracker",
}

// generateKeys generates all cryptographic keys needed for the stack.
func generateKeys(tempDir string) error {
	keysDir := filepath.Join(tempDir, "generated", "keys")

	// Generate Ed25519 keys for services
	for _, svc := range serviceKeys {
		if err := generateEd25519Key(keysDir, svc); err != nil {
			return fmt.Errorf("generate key for %s: %w", svc, err)
		}
	}

	// Extract EVM keys from deployed-addresses.json
	deployedAddresses, err := loadDeployedAddresses(tempDir)
	if err != nil {
		return fmt.Errorf("load deployed addresses: %w", err)
	}

	if err := extractEVMKeys(keysDir, deployedAddresses); err != nil {
		return fmt.Errorf("extract EVM keys: %w", err)
	}

	return nil
}

// generateEd25519Key generates an Ed25519 key pair and saves it in PEM format.
func generateEd25519Key(keysDir, name string) error {
	// Generate key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// Encode private key in PKCS#8 format
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}

	// Write private key PEM
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})
	privPath := filepath.Join(keysDir, name+".pem")
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Encode public key
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}

	// Write public key PEM
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})
	pubPath := filepath.Join(keysDir, name+".pub")
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// deployedAddresses represents the structure of deployed-addresses.json
type deployedAddresses struct {
	ChainID  int `json:"chainId"`
	Deployer struct {
		Address    string `json:"address"`
		PrivateKey string `json:"privateKey"`
	} `json:"deployer"`
	Payer struct {
		Address    string `json:"address"`
		PrivateKey string `json:"privateKey"`
	} `json:"payer"`
	Contracts map[string]string `json:"contracts"`
}

// loadDeployedAddresses reads the deployed-addresses.json file.
func loadDeployedAddresses(tempDir string) (*deployedAddresses, error) {
	path := filepath.Join(tempDir, "systems", "blockchain", "state", "deployed-addresses.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var addr deployedAddresses
	if err := json.Unmarshal(data, &addr); err != nil {
		return nil, err
	}

	return &addr, nil
}

// extractEVMKeys extracts EVM private keys from deployed addresses.
func extractEVMKeys(keysDir string, addr *deployedAddresses) error {
	// payer-key.hex: Raw hex private key (without 0x prefix)
	payerKey := strings.TrimPrefix(addr.Payer.PrivateKey, "0x")
	payerPath := filepath.Join(keysDir, "payer-key.hex")
	if err := os.WriteFile(payerPath, []byte(payerKey), 0600); err != nil {
		return fmt.Errorf("write payer key: %w", err)
	}

	// owner-wallet.hex: Piri wallet format (hex-encoded JSON)
	deployerKey := strings.TrimPrefix(addr.Deployer.PrivateKey, "0x")
	deployerKeyBytes, err := hex.DecodeString(deployerKey)
	if err != nil {
		return fmt.Errorf("decode deployer key: %w", err)
	}

	// Create piri wallet JSON: {"Type":"delegated","PrivateKey":"<base64>"}
	walletJSON := fmt.Sprintf(`{"Type":"delegated","PrivateKey":"%s"}`,
		base64.StdEncoding.EncodeToString(deployerKeyBytes))

	// Hex-encode the JSON
	walletHex := hex.EncodeToString([]byte(walletJSON))
	walletPath := filepath.Join(keysDir, "owner-wallet.hex")
	if err := os.WriteFile(walletPath, []byte(walletHex), 0600); err != nil {
		return fmt.Errorf("write owner wallet: %w", err)
	}

	return nil
}

package generator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"
)

const (
	defaultMinFileSize = int64(256 * 1024)       // 256 KiB
	defaultMaxFileSize = int64(32 * 1024 * 1024) // 32 MiB
)

// Config holds generator configuration
type Config struct {
	Seed        int64  // 0 = use time.Now().UnixNano()
	MinFileSize string // e.g., "256KB" (default)
	MaxFileSize string // e.g., "32MB" (default)
	BaseDir     string // e.g., "/tmp"
}

// Generator creates reproducible random directory trees
type Generator struct {
	seed        int64
	baseDir     string
	minFileSize int64
	maxFileSize int64
	counter     atomic.Int64 // increments each Generate() call for unique data
}

// GeneratedData represents the result of generating test data
type GeneratedData struct {
	Path      string // Root directory path containing generated files
	SizeBytes int64  // Total size of generated data
	Hash      string // SHA256 hash of all data (deterministic order)
	FileCount int    // Number of files generated
	DirCount  int    // Number of directories created
	Seed      int64  // Seed used for generation
}

// NewGenerator creates a new seeded data generator
func NewGenerator(cfg Config) (*Generator, error) {
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	minFileSize := defaultMinFileSize
	if cfg.MinFileSize != "" {
		parsed, err := ParseByteSize(cfg.MinFileSize)
		if err != nil {
			return nil, fmt.Errorf("invalid min_file_size: %w", err)
		}
		minFileSize = parsed
		// Clamp to avoid millions of tiny files
		if minFileSize < defaultMinFileSize {
			minFileSize = defaultMinFileSize
		}
	}

	maxFileSize := defaultMaxFileSize
	if cfg.MaxFileSize != "" {
		parsed, err := ParseByteSize(cfg.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("invalid max_file_size: %w", err)
		}
		maxFileSize = parsed
	}

	if minFileSize > maxFileSize {
		return nil, fmt.Errorf("min_file_size (%d) cannot exceed max_file_size (%d)", minFileSize, maxFileSize)
	}

	baseDir := cfg.BaseDir
	if baseDir == "" {
		baseDir = os.TempDir()
	}

	return &Generator{
		seed:        seed,
		baseDir:     baseDir,
		minFileSize: minFileSize,
		maxFileSize: maxFileSize,
	}, nil
}

// Generate creates a reproducible random directory tree with the specified total size
func (g *Generator) Generate(totalSize int64) (*GeneratedData, error) {
	if totalSize < 2 {
		return nil, fmt.Errorf("size too small to place files in both root and a subdirectory")
	}

	// Increment counter to get unique seed for this generation
	// This ensures each Generate() call produces unique data while remaining reproducible
	// (same base seed + same counter = same output)
	callNum := g.counter.Add(1)
	effectiveSeed := g.seed + callNum

	// Create a new RNG for this generation using the effective seed
	rng := rand.New(rand.NewSource(effectiveSeed))

	// Create unique root directory
	dirName := fmt.Sprintf("stress-data-%d-%d-%s", g.seed, callNum, g.randToken(rng, 6))
	rootPath := filepath.Join(g.baseDir, dirName)

	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	gen := &genState{
		rng:         rng,
		root:        rootPath,
		remaining:   totalSize,
		minFileSize: g.minFileSize,
		maxFileSize: g.maxFileSize,
		buf:         make([]byte, chooseBufferSize(g.maxFileSize)),
	}

	if err := gen.generate(); err != nil {
		os.RemoveAll(rootPath)
		return nil, err
	}

	// Compute hash over all files in deterministic order
	hash, err := hashDirectory(rootPath)
	if err != nil {
		os.RemoveAll(rootPath)
		return nil, fmt.Errorf("failed to hash directory: %w", err)
	}

	return &GeneratedData{
		Path:      rootPath,
		SizeBytes: gen.written,
		Hash:      hash,
		FileCount: gen.fileCount,
		DirCount:  gen.dirCount,
		Seed:      g.seed,
	}, nil
}

// Cleanup removes generated data
func (g *Generator) Cleanup(data *GeneratedData) error {
	if data == nil || data.Path == "" {
		return nil
	}
	return os.RemoveAll(data.Path)
}

// CleanupPath removes data at a specific path
func (g *Generator) CleanupPath(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

// randToken generates a random token of the specified length
func (g *Generator) randToken(rng *rand.Rand, length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = letters[rng.Intn(len(letters))]
	}
	return string(out)
}

// genState holds the state during directory tree generation
type genState struct {
	rng         *rand.Rand
	root        string
	remaining   int64
	written     int64
	fileCount   int
	dirCount    int
	minFileSize int64
	maxFileSize int64
	buf         []byte
}

// generate creates the directory tree structure (ported from randdir)
func (g *genState) generate() error {
	// Always place one file at the root and another inside a subdirectory
	rootFileSize := g.takeFileSize(g.remaining / 2)
	if err := g.writeFile(g.root, rootFileSize); err != nil {
		return err
	}

	subdirPath, err := g.makeSubdir(g.root)
	if err != nil {
		return err
	}
	if err := g.writeFile(subdirPath, g.takeFileSize(g.remaining)); err != nil {
		return err
	}

	directories := []string{g.root, subdirPath}

	for g.remaining > 0 {
		currentDir := directories[g.rng.Intn(len(directories))]

		// Occasionally branch deeper, biased by remaining space (35% probability)
		if g.remaining > g.minFileSize*2 && g.rng.Float64() < 0.35 {
			newDir, err := g.makeSubdir(currentDir)
			if err != nil {
				return err
			}
			directories = append(directories, newDir)
			continue
		}

		if err := g.writeFile(currentDir, g.takeFileSize(g.remaining)); err != nil {
			return err
		}
	}

	return nil
}

func (g *genState) writeFile(dir string, size int64) error {
	if size <= 0 {
		return nil
	}

	g.fileCount++
	fileName := fmt.Sprintf("file_%03d_%s.bin", g.fileCount, g.randToken(4))
	fullPath := filepath.Join(dir, fileName)

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, len(g.buf))
	remaining := size
	for remaining > 0 {
		chunk := int64(len(g.buf))
		if chunk > remaining {
			chunk = remaining
		}
		if _, err := g.rng.Read(g.buf[:chunk]); err != nil {
			return err
		}
		if _, err := w.Write(g.buf[:chunk]); err != nil {
			return err
		}
		remaining -= chunk
	}
	if err := w.Flush(); err != nil {
		return err
	}

	g.remaining -= size
	g.written += size
	return nil
}

func (g *genState) makeSubdir(parent string) (string, error) {
	g.dirCount++
	dirName := fmt.Sprintf("dir_%03d_%s", g.dirCount, g.randToken(3))
	path := filepath.Join(parent, dirName)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

func (g *genState) takeFileSize(maxBytes int64) int64 {
	if maxBytes <= 0 {
		return 0
	}
	maxCandidate := maxBytes
	if maxCandidate > g.maxFileSize {
		maxCandidate = g.maxFileSize
	}

	minCandidate := g.minFileSize
	if minCandidate > maxCandidate {
		minCandidate = maxCandidate
	}

	size := minCandidate
	if maxCandidate > minCandidate {
		size = g.rng.Int63n(maxCandidate-minCandidate+1) + minCandidate
	}
	return size
}

func (g *genState) randToken(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = letters[g.rng.Intn(len(letters))]
	}
	return string(out)
}

// hashDirectory computes a SHA256 hash of all files in a directory in deterministic order
func hashDirectory(dirPath string) (string, error) {
	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort files for deterministic ordering
	sort.Strings(files)

	hasher := sha256.New()
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(hasher, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashDirectory computes a hash of all files in a directory (exported for external use)
func HashDirectory(dirPath string) (string, int64, error) {
	var files []string
	var totalSize int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}

	// Sort files for deterministic ordering
	sort.Strings(files)

	hasher := sha256.New()
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return "", 0, err
		}
		if _, err := io.Copy(hasher, f); err != nil {
			f.Close()
			return "", 0, err
		}
		f.Close()
	}

	return hex.EncodeToString(hasher.Sum(nil)), totalSize, nil
}

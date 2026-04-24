package update

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

func ParseChecksums(data []byte) (map[string]string, error) {
	checksums := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("parse checksums: malformed line %q", line)
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != sha256.Size*2 {
			return nil, fmt.Errorf("parse checksums: invalid sha256 digest for %q", fields[1])
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return nil, fmt.Errorf("parse checksums: invalid sha256 digest for %q: %w", fields[1], err)
		}
		name := strings.TrimPrefix(fields[1], "*")
		checksums[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse checksums: %w", err)
	}
	return checksums, nil
}

func ExpectedChecksum(data []byte, assetName string) (string, error) {
	checksums, err := ParseChecksums(data)
	if err != nil {
		return "", err
	}
	expected, ok := checksums[assetName]
	if !ok {
		return "", fmt.Errorf("checksums.txt does not contain %q", assetName)
	}
	return expected, nil
}

func VerifySHA256(data []byte, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("invalid expected sha256 digest")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return fmt.Errorf("invalid expected sha256 digest: %w", err)
	}
	actualBytes := sha256.Sum256(data)
	actual := hex.EncodeToString(actualBytes[:])
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

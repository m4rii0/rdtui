package update

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestExpectedChecksum(t *testing.T) {
	data := []byte("abc123  rdtui-linux-amd64\n")
	_, err := ExpectedChecksum(data, "rdtui-linux-amd64")
	if err == nil {
		t.Fatal("expected invalid digest error")
	}

	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("asset")))
	got, err := ExpectedChecksum([]byte(digest+"  *rdtui-linux-amd64\n"), "rdtui-linux-amd64")
	if err != nil {
		t.Fatalf("ExpectedChecksum returned error: %v", err)
	}
	if got != digest {
		t.Fatalf("ExpectedChecksum = %q, want %q", got, digest)
	}
}

func TestExpectedChecksumMissing(t *testing.T) {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("asset")))
	if _, err := ExpectedChecksum([]byte(digest+"  other\n"), "rdtui-linux-amd64"); err == nil {
		t.Fatal("expected missing checksum error")
	}
}

func TestVerifySHA256(t *testing.T) {
	asset := []byte("asset")
	digest := fmt.Sprintf("%x", sha256.Sum256(asset))
	if err := VerifySHA256(asset, digest); err != nil {
		t.Fatalf("VerifySHA256 returned error: %v", err)
	}
	if err := VerifySHA256([]byte("different"), digest); err == nil {
		t.Fatal("expected mismatch error")
	}
}

package results

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

// ID obfuscation provides an optional privacy layer for test result URLs.
// When enabled, the telemetry endpoint returns an obfuscated ULID that must
// be deobfuscated before looking up the result.
//
// This is NOT cryptographically secure — it prevents casual ID guessing,
// matching the behavior of the PHP version's idObfuscation.php.

var (
	obfuscationSalt     uint32
	obfuscationSaltOnce sync.Once
	obfuscationSaltErr  error
)

const obfuscationSaltFile = "idObfuscation_salt.bin"

func getOrCreateObfuscationSalt() (uint32, error) {
	obfuscationSaltOnce.Do(func() {
		data, err := os.ReadFile(obfuscationSaltFile)
		if err == nil && len(data) == 4 {
			obfuscationSalt = binary.LittleEndian.Uint32(data)
			return
		}

		saltBytes := make([]byte, 4)
		if _, err := rand.Read(saltBytes); err != nil {
			obfuscationSaltErr = fmt.Errorf("failed to generate obfuscation salt: %w", err)
			return
		}
		obfuscationSalt = binary.LittleEndian.Uint32(saltBytes)

		dir := filepath.Dir(obfuscationSaltFile)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Warnf("Could not create directory for obfuscation salt file: %s", err)
			}
		}
		if err := os.WriteFile(obfuscationSaltFile, saltBytes, 0644); err != nil {
			log.Warnf("Could not save obfuscation salt file: %s", err)
		}
	})

	return obfuscationSalt, obfuscationSaltErr
}

// obfuscateBytes applies reversible transform on ULID bytes:
// XOR the first 4 bytes with the salt (simple but effective for casual privacy)
func obfuscateBytes(data []byte) []byte {
	salt, err := getOrCreateObfuscationSalt()
	if err != nil || len(data) < 4 {
		return data
	}
	result := make([]byte, len(data))
	copy(result, data)
	val := binary.LittleEndian.Uint32(result[:4])
	val ^= salt
	binary.LittleEndian.PutUint32(result[:4], val)
	return result
}

// deobfuscateBytes reverses obfuscateBytes (XOR is self-inverse)
var deobfuscateBytes = obfuscateBytes

// ObfuscateULID transforms a ULID string to its obfuscated (base64) form
func ObfuscateULID(id string) string {
	parsed, err := ulid.Parse(id)
	if err != nil {
		return id
	}
	obfuscated := obfuscateBytes(parsed[:])
	return base64.RawURLEncoding.EncodeToString(obfuscated)
}

// DeobfuscateULID reverses ULID obfuscation
func DeobfuscateULID(obfuscated string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(obfuscated)
	if err != nil {
		return "", fmt.Errorf("invalid obfuscated ID encoding: %w", err)
	}
	if len(data) != 16 {
		return "", fmt.Errorf("invalid obfuscated ID length: %d", len(data))
	}
	deobfuscated := deobfuscateBytes(data)
	var id ulid.ULID
	copy(id[:], deobfuscated)
	return id.String(), nil
}

// ResolveID takes an ID string and returns the database ULID.
// It tries the raw input first, then attempts deobfuscation.
func ResolveID(id string) string {
	// First try: use as-is (plain ULID)
	if _, err := ulid.Parse(id); err == nil {
		return id
	}

	// Second try: deobfuscate
	if deobfuscated, err := DeobfuscateULID(id); err == nil {
		return deobfuscated
	}

	return id
}

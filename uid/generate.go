package uid

import (
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"math/big"
	"regexp"
)

// RootUID is the default UID root prefix used by GenerateUID
// (Medical Connections / same root as pydicom).
const RootUID = "1.2.826.0.1.3680043.8.498."

const maxPrefixLength = 54

var validUIDPrefix = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))*\.$`)

type generateConfig struct {
	prefix     string
	useUUID    bool
	entropy    []string
	hasEntropy bool
}

// GenerateOption configures GenerateUID.
type GenerateOption func(*generateConfig)

// WithPrefix sets the UID prefix (must end with '.' and be ≤ 54 characters).
func WithPrefix(prefix string) GenerateOption {
	return func(c *generateConfig) {
		c.prefix = prefix
		c.useUUID = false
	}
}

// WithUUIDPrefix generates a UID of the form 2.25.<uuid4-as-int>
// (pydicom generate_uid(prefix=None)).
func WithUUIDPrefix() GenerateOption {
	return func(c *generateConfig) {
		c.useUUID = true
		c.prefix = ""
	}
}

// WithEntropy appends a deterministic SHA-512-derived suffix from the joined
// entropy sources (pydicom entropy_srcs). Ignored when WithUUIDPrefix is set.
func WithEntropy(srcs ...string) GenerateOption {
	return func(c *generateConfig) {
		c.entropy = append([]string(nil), srcs...)
		c.hasEntropy = true
	}
}

// GenerateUID returns a DICOM UID of at most 64 characters.
//
// By default the UID starts with [RootUID] and a cryptographically random
// numeric suffix. See WithPrefix, WithUUIDPrefix, and WithEntropy.
func GenerateUID(opts ...GenerateOption) (UID, error) {
	cfg := generateConfig{prefix: RootUID}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.useUUID {
		u, err := randomUUIDv4Int()
		if err != nil {
			return "", fmt.Errorf("uid: generate uuid: %w", err)
		}
		return UID("2.25." + u.String()), nil
	}

	prefix := cfg.prefix
	if len(prefix) > maxPrefixLength {
		return "", fmt.Errorf("uid: prefix should be no more than %d characters long", maxPrefixLength)
	}
	if !validUIDPrefix.MatchString(prefix) {
		return "", fmt.Errorf("uid: prefix %q is not valid for use with a UID", prefix)
	}

	if !cfg.hasEntropy {
		maximum := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(64-len(prefix))), nil)
		n, err := rand.Int(rand.Reader, maximum)
		if err != nil {
			return "", fmt.Errorf("uid: random suffix: %w", err)
		}
		s := prefix + n.String()
		if len(s) > 64 {
			s = s[:64]
		}
		return UID(s), nil
	}

	sum := sha512.Sum512([]byte(joinEntropy(cfg.entropy)))
	n := new(big.Int).SetBytes(sum[:])
	s := prefix + n.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return UID(s), nil
}

// MustGenerateUID is like GenerateUID but panics on error.
func MustGenerateUID(opts ...GenerateOption) UID {
	u, err := GenerateUID(opts...)
	if err != nil {
		panic(err)
	}
	return u
}

func joinEntropy(srcs []string) string {
	if len(srcs) == 0 {
		return ""
	}
	n := 0
	for _, s := range srcs {
		n += len(s)
	}
	b := make([]byte, 0, n)
	for _, s := range srcs {
		b = append(b, s...)
	}
	return string(b)
}

func randomUUIDv4Int() (*big.Int, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return new(big.Int).SetBytes(b[:]), nil
}

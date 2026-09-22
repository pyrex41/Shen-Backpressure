package main

// sign_report.go — W5.3. `sb sign-report` fills the discharge report's
// reserved `signature` field.
//
// Two modes:
//
//   - key file (default). Ed25519 from the standard library, keys on
//     disk. No network, no registry, no daemon: a signature can be
//     produced and checked on an air-gapped machine, which matters
//     because the whole point of the certificate is that checking it
//     needs nothing the producer had.
//
//   - --cosign. Shells out to a `cosign` binary if one is on PATH,
//     using keyless signing so the signer is the CI identity rather
//     than a key someone has to rotate. Deliberately a subprocess and
//     not a Go dependency: sigstore's module graph is large, and a
//     verifier that cannot be built offline defeats the purpose.
//
// What is signed is the canonical byte derivation in canonical.go,
// not the file: see that file and docs/TRUST-MODEL.md.
//
// What a signature means: "the holder of this key asserts that these
// claims were produced by this pipeline." It says nothing about
// whether the claims are true — that is what `sb verify-report`
// re-derives. Signature and verification are independent; requiring
// both is what `--require-sig` is for.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Signature algorithm / mode identifiers recorded in the report.
const (
	SigAlgorithmEd25519 = "ed25519"
	SigAlgorithmCosign  = "cosign"

	SigModeKeyFile = "key-file"
	SigModeCosign  = "cosign-keyless"
)

// DefaultSigningKeyPath is where `sb sign-report --generate-key` puts
// a fresh private key. Under .sb/ and therefore gitignored: committing
// a signing key would make every "signed" report worthless.
const DefaultSigningKeyPath = ".sb/signing-key.json"

// ed25519KeyFile is the on-disk key format. Both halves are stored so
// the public key can be published without re-deriving it; the private
// half is what must not leave the machine.
type ed25519KeyFile struct {
	Algorithm  string `json:"algorithm"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

func cmdSignReport(args []string) {
	fs := flag.NewFlagSet("sign-report", flag.ExitOnError)
	in := fs.String("in", DischargeReportPath, "discharge report to sign (signed in place)")
	keyPath := fs.String("key", DefaultSigningKeyPath, "ed25519 key file")
	generate := fs.Bool("generate-key", false, "generate a fresh ed25519 key at --key and exit")
	useCosign := fs.Bool("cosign", false, "sign with the cosign binary (keyless) instead of a key file")
	out := fs.String("out", "", "write the signed report here instead of over --in")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `sb sign-report — Sign a discharge report

Usage: sb sign-report [flags]

Writes a detached signature into the report's `+"`signature`"+` field. The
signature covers the report's canonical bytes (%s):
the document with its own signature member removed, re-encoded with
object keys sorted and no insignificant whitespace. Whitespace, key
order, and the signature itself therefore do not affect it.

Modes:

  (default)  ed25519 with a key file. Generate one with:
                 sb sign-report --generate-key
             which writes %s (gitignored). Publish the
             public_key line; keep the private half off the network.

  --cosign   Keyless signing via the cosign binary, if one is on PATH.
             The signer is the ambient CI identity (an OIDC token),
             so there is no key to manage or rotate. cosign is not a
             Go dependency of sb — it is invoked as a subprocess, and
             its absence is reported rather than worked around.

Verify with:

  sb verify-report --require-sig

Flags:
`, CanonicalizationName, DefaultSigningKeyPath)
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *generate {
		if err := generateSigningKey(*keyPath); err != nil {
			fmt.Fprintf(os.Stderr, "sb sign-report: %v\n", err)
			os.Exit(1)
		}
		return
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb sign-report: %v\n", err)
		os.Exit(1)
	}
	canonical, err := CanonicalReportBytes(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb sign-report: %v\n", err)
		os.Exit(1)
	}

	var sig *DischargeSignature
	if *useCosign {
		sig, err = cosignSign(canonical)
	} else {
		sig, err = ed25519Sign(canonical, *keyPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sb sign-report: %v\n", err)
		os.Exit(1)
	}

	var r DischargeReport
	if err := json.Unmarshal(raw, &r); err != nil {
		fmt.Fprintf(os.Stderr, "sb sign-report: parse %s: %v\n", *in, err)
		os.Exit(1)
	}
	r.Signature = sig

	target := *out
	if target == "" {
		target = *in
	}
	if err := writeDischarge(target, &r); err != nil {
		fmt.Fprintf(os.Stderr, "sb sign-report: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "sb sign-report: signed %s (%s, key %s)\n",
		target, sig.Algorithm, abbreviate(sig.KeyID))
}

// generateSigningKey writes a fresh ed25519 keypair to path.
func generateSigningKey(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; delete it deliberately rather than overwriting a signing key", path)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	kf := ed25519KeyFile{
		Algorithm:  SigAlgorithmEd25519,
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: base64.StdEncoding.EncodeToString(priv),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Comment:    "sb discharge-report signing key. Keep the private half off the network and out of git.",
	}
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 0600: the private half is in this file.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sb sign-report: wrote %s\n", path)
	fmt.Fprintf(os.Stderr, "  public key: %s\n", kf.PublicKey)
	fmt.Fprintf(os.Stderr, "  Publish the public key; never commit this file.\n")
	return nil
}

// loadSigningKey reads a key file. A file with only a public half is
// valid for verification and rejected for signing by the caller.
func loadSigningKey(path string) (*ed25519KeyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no signing key at %s — run `sb sign-report --generate-key`", path)
		}
		return nil, err
	}
	var kf ed25519KeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if kf.Algorithm != "" && kf.Algorithm != SigAlgorithmEd25519 {
		return nil, fmt.Errorf("%s: unsupported algorithm %q", path, kf.Algorithm)
	}
	return &kf, nil
}

// ed25519Sign signs canonical bytes with the key file at keyPath.
func ed25519Sign(canonical []byte, keyPath string) (*DischargeSignature, error) {
	kf, err := loadSigningKey(keyPath)
	if err != nil {
		return nil, err
	}
	if kf.PrivateKey == "" {
		return nil, fmt.Errorf("%s holds only a public key; signing needs the private half", keyPath)
	}
	privBytes, err := base64.StdEncoding.DecodeString(kf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%s: private key is not base64: %w", keyPath, err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s: private key is %d bytes, want %d", keyPath, len(privBytes), ed25519.PrivateKeySize)
	}
	priv := ed25519.PrivateKey(privBytes)
	sig := ed25519.Sign(priv, canonical)

	pub := kf.PublicKey
	if pub == "" {
		pub = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	}
	return &DischargeSignature{
		Algorithm:        SigAlgorithmEd25519,
		KeyID:            pub,
		Value:            base64.StdEncoding.EncodeToString(sig),
		Canonicalization: CanonicalizationName,
		SignedAt:         time.Now().UTC().Format(time.RFC3339),
		Mode:             SigModeKeyFile,
	}, nil
}

// VerifySignature checks a report's signature against its canonical
// bytes. Returns a human-readable description of what was verified on
// success.
func VerifySignature(raw []byte, sig *DischargeSignature, cosignIdentity, cosignIssuer string) (string, error) {
	if sig == nil {
		return "", fmt.Errorf("report carries no signature")
	}
	if sig.Canonicalization != "" && sig.Canonicalization != CanonicalizationName {
		return "", fmt.Errorf("signature uses canonicalization %q; this sb produces %q",
			sig.Canonicalization, CanonicalizationName)
	}
	canonical, err := CanonicalReportBytes(raw)
	if err != nil {
		return "", err
	}
	switch sig.Algorithm {
	case SigAlgorithmEd25519:
		pub, err := base64.StdEncoding.DecodeString(sig.KeyID)
		if err != nil {
			return "", fmt.Errorf("key_id is not base64: %w", err)
		}
		if len(pub) != ed25519.PublicKeySize {
			return "", fmt.Errorf("key_id is %d bytes, want an %d-byte ed25519 public key", len(pub), ed25519.PublicKeySize)
		}
		value, err := base64.StdEncoding.DecodeString(sig.Value)
		if err != nil {
			return "", fmt.Errorf("signature value is not base64: %w", err)
		}
		if !ed25519.Verify(ed25519.PublicKey(pub), canonical, value) {
			return "", fmt.Errorf("ed25519 signature does not verify against the report's canonical bytes")
		}
		return "ed25519 signature by " + abbreviate(sig.KeyID), nil
	case SigAlgorithmCosign:
		return cosignVerify(canonical, sig, cosignIdentity, cosignIssuer)
	default:
		return "", fmt.Errorf("unknown signature algorithm %q", sig.Algorithm)
	}
}

func abbreviate(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:16] + "…"
}

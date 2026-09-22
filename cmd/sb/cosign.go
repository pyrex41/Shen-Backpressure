package main

// cosign.go — W5.3, the documented keyless mode.
//
// cosign is invoked as a subprocess and is never a Go dependency of
// sb. That is a deliberate trade: keyless signing binds the signature
// to a CI identity rather than to a key someone must rotate, which is
// what the roadmap asks for, but it also needs an OIDC token, a
// Fulcio certificate, and a Rekor entry — none of which a verifier on
// an air-gapped machine can obtain. So ed25519 with a key file stays
// the default, and cosign is opt-in for pipelines that already have
// the identity.
//
// The bundle cosign produces is stored base64-encoded in the
// signature's `value`, so the report stays a single self-contained
// file. Verification writes it back out to a temp file and hands it
// to `cosign verify-blob`.

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cosignBinary is looked up on PATH at call time, so a pipeline that
// installs cosign mid-run still works.
const cosignBinary = "cosign"

// cosignSign signs canonical bytes with `cosign sign-blob` in keyless
// mode and returns the signature to embed in the report.
func cosignSign(canonical []byte) (*DischargeSignature, error) {
	bin, err := exec.LookPath(cosignBinary)
	if err != nil {
		return nil, fmt.Errorf("--cosign needs a `cosign` binary on PATH (not found). " +
			"Install cosign, or drop --cosign to sign with an ed25519 key file")
	}

	dir, err := os.MkdirTemp("", "sb-cosign-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	blob := filepath.Join(dir, "report.canonical.json")
	if err := os.WriteFile(blob, canonical, 0o600); err != nil {
		return nil, err
	}
	bundlePath := filepath.Join(dir, "bundle.json")

	cmd := exec.Command(bin, "sign-blob", "--yes", "--bundle", bundlePath, blob)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cosign sign-blob: %w", err)
	}
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("cosign produced no bundle: %w", err)
	}

	return &DischargeSignature{
		Algorithm:        SigAlgorithmCosign,
		KeyID:            cosignIdentityFromBundle(bundle),
		Value:            base64.StdEncoding.EncodeToString(bundle),
		Canonicalization: CanonicalizationName,
		SignedAt:         time.Now().UTC().Format(time.RFC3339),
		Mode:             SigModeCosign,
	}, nil
}

// cosignVerify checks a cosign bundle over the canonical bytes.
//
// Keyless verification requires the caller to state which identity it
// expects: a signature from *some* identity proves nothing, since
// anyone with an OIDC account can produce one. `sb verify-report`
// takes --cosign-identity and --cosign-issuer for this, and refuses
// rather than guessing.
func cosignVerify(canonical []byte, sig *DischargeSignature, identity, issuer string) (string, error) {
	bin, err := exec.LookPath(cosignBinary)
	if err != nil {
		return "", fmt.Errorf("report is cosign-signed but no `cosign` binary is on PATH")
	}
	if identity == "" || issuer == "" {
		return "", fmt.Errorf("verifying a cosign signature needs --cosign-identity and --cosign-issuer: " +
			"a keyless signature says who signed, and only you can say who should have")
	}
	bundle, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return "", fmt.Errorf("cosign bundle is not base64: %w", err)
	}

	dir, err := os.MkdirTemp("", "sb-cosign-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	blob := filepath.Join(dir, "report.canonical.json")
	if err := os.WriteFile(blob, canonical, 0o600); err != nil {
		return "", err
	}
	bundlePath := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		return "", err
	}

	cmd := exec.Command(bin, "verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", issuer,
		blob)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cosign verify-blob failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return "cosign keyless signature by " + identity + " (issuer " + issuer + ")", nil
}

// cosignIdentityFromBundle names the signer recorded in a bundle.
//
// Keyless identity lives in a Fulcio certificate inside the bundle,
// and parsing X.509 out of an evolving bundle format to print a
// nicer string is not worth the coupling: verification takes the
// expected identity from the caller anyway (see cosignVerify), so the
// stored key_id is a mode label, not a trust input.
func cosignIdentityFromBundle(bundle []byte) string {
	return "cosign-keyless"
}
